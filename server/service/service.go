package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/poodbooq/bitburst_server/logger"
	"github.com/poodbooq/bitburst_server/models"
	"github.com/poodbooq/bitburst_server/postgres"
	"github.com/prometheus/common/log"
	"net/http"
	"sync"
	"time"
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

	inputIDsCh      chan int
	refreshIDsCh    chan int
	upsertObjectsCh chan models.Object
	deleteObjectsCh chan int

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
			database:        db,
			log:             log,
			cfg:             cfg,
			router:          httprouter.New(),
			httpClient:      client,
			inputIDsCh:      make(chan int, cfg.MaxObjectsPerRequest),
			refreshIDsCh:    make(chan int, cfg.MaxObjectsPerRequest),
			upsertObjectsCh: make(chan models.Object, cfg.MaxObjectsPerRequest),
			deleteObjectsCh: make(chan int, cfg.MaxObjectsPerRequest),
			timers: &timer{
				mu:   new(sync.Mutex),
				byID: make(map[int]*time.Timer),
			},
		}
	})

	return singleton
}

func (s *service) Run(ctx context.Context) {
	if s.isRunning {
		return
	}
	s.isRunning = true

	go s.listenCallback(ctx)
	go s.upsertObjects(ctx)
	go s.getObjectsInfo(ctx)
	go s.handleObjectsExpiration(ctx)
	go s.deleteObjects(ctx)
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
	close(s.refreshIDsCh)
	close(s.inputIDsCh)
	close(s.deleteObjectsCh)
	close(s.upsertObjectsCh)
}

func (s *service) deleteObjects(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.deleteObjectsCh:
			err := s.database.DeleteObjectByID(ctx, id)
			if err != nil {
				s.log.Error(err)
			} else {
				s.log.Debug("deleted object with id %v", id)
			}
		}
	}
}

func (s *service) handleObjectsExpiration(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.refreshIDsCh:
			s.timers.mu.Lock()
			if timer, ok := s.timers.byID[id]; !ok {
				s.timers.byID[id] = time.NewTimer(time.Second * time.Duration(s.cfg.RetentionPolicySec))
				s.log.Debug("set new timer for id %v", id)
				s.timers.mu.Unlock()
				go func(id int) {
					<-s.timers.byID[id].C
					s.log.Debug("expired object with id %v, sending to delete chan", id)
					s.timers.mu.Lock()
					delete(s.timers.byID, id)
					s.deleteObjectsCh <- id
					s.timers.mu.Unlock()
				}(id)
			} else {
				if !timer.Stop() {
					<-timer.C
				}
				s.log.Debug("received id %v before expiration, refreshing timer", id)
				timer.Reset(time.Second * time.Duration(s.cfg.RetentionPolicySec))
				s.timers.mu.Unlock()
			}
		}
	}
}

func (s *service) getObjectsInfo(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.inputIDsCh:
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
					log.Error(err)
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
				s.upsertObjectsCh <- info
				s.refreshIDsCh <- info.ID
			}(ctx, id)
		}
	}
}

func (s *service) upsertObjects(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case obj := <-s.upsertObjectsCh:
			s.log.Debug("upserting object: id=%v, online=%v", obj.ID, obj.Online)
			if err := s.database.UpsertObject(ctx, obj); err != nil {
				s.log.Error(err)
			}
		}
	}
}

func (s *service) listenCallback(ctx context.Context) {
	s.router.POST("/callback", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		dec := json.NewDecoder(r.Body)
		var input models.ObjectsInput
		err := dec.Decode(&input)
		_ = r.Body.Close()
		if err != nil {
			s.log.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
		} else {
			go func() {
				for i := range input.ObjectIDs {
					s.log.Debug("retrieved id: %v", input.ObjectIDs[i])
					s.inputIDsCh <- input.ObjectIDs[i]
				}
			}()
		}
	})
}
