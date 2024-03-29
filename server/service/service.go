package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/poodbooq/bitburst_server/logger"
	"github.com/poodbooq/bitburst_server/models"
	"github.com/poodbooq/bitburst_server/postgres"
)

type Config struct {
	MaxObjectsPerRequest int
	RetentionPolicySec   int
	HTTP                 HttpConfig
}

type HttpConfig struct {
	ListenPort string
	TesterPort string
	TesterHost string
	TimeoutSec int
}

type timer struct {
	mu   *sync.Mutex
	byID map[int]*time.Timer
}

type service struct {
	database   postgres.Postgres
	log        logger.Logger
	cfg        Config
	router     *httprouter.Router
	httpClient *http.Client

	isRunning bool

	inputCh      chan int
	expirationCh chan models.Object
	upsertCh     chan models.Object
	deleteCh     chan int

	timers *timer
}

var (
	singleton *service
	once      = new(sync.Once)
)

func Load(db postgres.Postgres, log logger.Logger, cfg Config) *service {
	once.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:    cfg.MaxObjectsPerRequest,
			MaxConnsPerHost: cfg.MaxObjectsPerRequest,
		}
		client := &http.Client{Timeout: time.Duration(cfg.HTTP.TimeoutSec) * time.Second, Transport: tr}
		singleton = &service{
			database:     db,
			log:          log,
			cfg:          cfg,
			router:       httprouter.New(),
			httpClient:   client,
			inputCh:      make(chan int, cfg.MaxObjectsPerRequest),
			expirationCh: make(chan models.Object, cfg.MaxObjectsPerRequest),
			upsertCh:     make(chan models.Object, cfg.MaxObjectsPerRequest),
			deleteCh:     make(chan int, cfg.MaxObjectsPerRequest),
			timers: &timer{
				mu:   new(sync.Mutex),
				byID: make(map[int]*time.Timer),
			},
		}
	})

	return singleton
}

func (s *service) Run(ctx context.Context) {
	if s.isRunning { // preventing multiple runs
		return
	}
	s.isRunning = true

	go s.coldStart(ctx)               // get all existing objects from database and handle their expirations if no object with such id came
	go s.handleCallbackRoute(ctx)     // listening requests with object ids from tester program and passing ids to input channel
	go s.retrieveObjects(ctx)         // reading input channel, retrieving objects' statuses and passing them to the channel depending on the object's status (online -> upsert && expire channels, offline -> delete channel)
	go s.handleUpsert(ctx)            // reading upsert channel, upserting incoming online objects
	go s.handleObjectsExpiration(ctx) // handle expire time for objects, that weren't received repeatedly for the predefined time
	go s.handleDelete(ctx)            // delete expired objects
	go func() { _ = http.ListenAndServe(fmt.Sprintf(":%v", s.cfg.HTTP.ListenPort), s.router) }()

	for {
		select {
		case <-ctx.Done():
			s.log.Debug("closing all channels")
			s.close()
		}
	}
}

func (s *service) close() {
	close(s.expirationCh)
	close(s.inputCh)
	close(s.deleteCh)
	close(s.upsertCh)
}

func (s *service) coldStart(ctx context.Context) {
	objs, err := s.database.GetAll(ctx)
	if err != nil {
		s.log.Error(err)
		return
	}
	for i := range objs {
		if objs[i].LastSeenAt != nil && (time.Now().UTC().Sub(*objs[i].LastSeenAt) > time.Second*time.Duration(s.cfg.RetentionPolicySec)) {
			s.deleteCh <- objs[i].ID
		} else {
			s.expirationCh <- objs[i]
		}
	}
}

func (s *service) handleDelete(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.deleteCh:
			go func(ctx context.Context, id int) {
				err := s.database.DeleteObjectByID(ctx, id)
				if err != nil {
					s.log.Error(err)
				} else {
					s.log.Debug("deleted object with id %v", id)
				}
			}(ctx, id)
		}
	}
}

func (s *service) handleObjectsExpiration(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case obj := <-s.expirationCh:
			s.timers.mu.Lock()
			if timer, ok := s.timers.byID[obj.ID]; !ok {
				now := time.Now().UTC()
				if obj.LastSeenAt != nil && now.Sub(*obj.LastSeenAt) < time.Second*time.Duration(s.cfg.RetentionPolicySec) {
					s.timers.byID[obj.ID] = time.NewTimer(time.Second*time.Duration(s.cfg.RetentionPolicySec) - now.Sub(*obj.LastSeenAt))
				} else {
					s.timers.byID[obj.ID] = time.NewTimer(time.Second * time.Duration(s.cfg.RetentionPolicySec))
				}
				s.log.Debug("set new timer for id %v", obj.ID)
				s.timers.mu.Unlock()
				go func(id int) {
					<-s.timers.byID[id].C
					s.log.Debug("expired object with id %v, sending to delete chan", id)
					s.timers.mu.Lock()
					delete(s.timers.byID, id)
					s.deleteCh <- id
					s.timers.mu.Unlock()
				}(obj.ID)
			} else {
				if !timer.Stop() {
					<-timer.C
				}
				s.log.Debug("received id %v before expiration, refreshing timer", obj.ID)
				timer.Reset(time.Second * time.Duration(s.cfg.RetentionPolicySec)) // refresh timer if id was received before expire
				s.timers.mu.Unlock()
			}
		}
	}
}

func (s *service) retrieveObjects(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.inputCh:
			go func(ctx context.Context, id int) {
				req, err := http.NewRequestWithContext(
					ctx,
					http.MethodGet,
					fmt.Sprintf("http://%s:%s/objects/%v", s.cfg.HTTP.TesterHost, s.cfg.HTTP.TesterPort, id),
					nil,
				)
				if err != nil {
					s.log.Error(err)
					return
				}
				s.log.Debug("requesting info by id=%v", id)
				resp, err := s.httpClient.Do(req)
				if err != nil {
					s.log.Error(err)
					return
				}
				var (
					info models.Object
					dec  = json.NewDecoder(resp.Body)
				)
				err = dec.Decode(&info)
				if errBodyClose := resp.Body.Close(); errBodyClose != nil {
					s.log.Error(err)
				}
				if err != nil {
					s.log.Error(err)
					return
				}
				s.log.Debug("got info for id=%v, online=%v", info.ID, info.Online)
				if info.Online {
					now := time.Now().UTC()
					info.LastSeenAt = &now
				}

				switch info.Online {
				case true:
					s.upsertCh <- info     // update or insert online objects
					s.expirationCh <- info // track expiration time
				case false:
					s.deleteCh <- info.ID // delete objects with offline status
				}
			}(ctx, id)
		}
	}
}

func (s *service) handleUpsert(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case obj := <-s.upsertCh:
			go func(ctx context.Context, obj models.Object) {
				s.log.Debug("upserting object: id=%v, online=%v", obj.ID, obj.Online)
				if err := s.database.UpsertObject(ctx, obj); err != nil {
					s.log.Error(err)
				}
			}(ctx, obj)
		}
	}
}

func (s *service) handleCallbackRoute(_ context.Context) {
	s.router.POST("/callback", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		dec := json.NewDecoder(r.Body)
		var input models.ObjectsInput
		err := dec.Decode(&input)
		if errBodyClose := r.Body.Close(); errBodyClose != nil {
			s.log.Error(errBodyClose)
		}
		if err != nil {
			s.log.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
		} else {
			go func() {
				for i := range input.ObjectIDs {
					s.log.Debug("retrieved id: %v", input.ObjectIDs[i])
					s.inputCh <- input.ObjectIDs[i]
				}
			}()
		}
	})
}
