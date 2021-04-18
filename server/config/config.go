package config

import (
	"github.com/pkg/errors"
	"github.com/poodbooq/bitburst_server/logger"
	"github.com/poodbooq/bitburst_server/service"
	"os"
	"strconv"
	"sync"

	"github.com/poodbooq/bitburst_server/postgres"
)

type config struct {
	Postgres postgres.Config
	Service  service.Config
	Logger   logger.Config
}

var (
	cfg              *config
	once             = new(sync.Once)
	errNoConfigFound = errors.New("no env config found")
)

func Load() (*config, error) {
	var err error
	once.Do(func() {
		cfg = &config{}
		cfg.Postgres, err = loadPostgresCfg()
		if err != nil {
			return
		}
		cfg.Logger, err = loadLoggerCfg()
		if err != nil {
			return
		}
		cfg.Service, err = loadServiceCfg()
		if err != nil {
			return
		}
	})
	return cfg, err
}

func loadServiceCfg() (service.Config, error) {
	var (
		serviceCfg service.Config
		err        error
	)
	maxObjStr, ok := os.LookupEnv("MAX_OBJECTS_PER_REQUEST")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	serviceCfg.MaxObjectsPerRequest, err = strconv.Atoi(maxObjStr)
	if err != nil {
		return service.Config{}, err
	}
	retentionStr, ok := os.LookupEnv("RETENTION_POLICY_SEC")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	serviceCfg.RetentionPolicySec, err = strconv.Atoi(retentionStr)
	if err != nil {
		return service.Config{}, err
	}
	serviceCfg.HTTP.ListenPort, ok = os.LookupEnv("LISTEN_PORT")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	serviceCfg.HTTP.TesterPort, ok = os.LookupEnv("TESTER_PORT")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	serviceCfg.HTTP.TesterHost, ok = os.LookupEnv("TESTER_HOST")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	timeoutStr, ok := os.LookupEnv("TIMEOUT_SEC")
	if !ok {
		return service.Config{}, errNoConfigFound
	}
	serviceCfg.HTTP.TimeoutSec, err = strconv.Atoi(timeoutStr)
	if err != nil {
		return service.Config{}, err
	}
	return serviceCfg, nil
}

func loadLoggerCfg() (logger.Config, error) {
	var (
		logCfg logger.Config
		err    error
	)
	if logCfgRaw, ok := os.LookupEnv("IS_PRODUCTION"); !ok {
		return logCfg, errNoConfigFound
	} else {
		logCfg.IsProduction, err = strconv.ParseBool(logCfgRaw)
		if err != nil {
			return logCfg, err
		}
	}
	return logCfg, nil
}

func loadPostgresCfg() (postgres.Config, error) {
	var (
		pgCfg = postgres.Config{}
		ok    bool
		err   error
	)
	if pgCfg.User, ok = os.LookupEnv("POSTGRES_USER"); !ok {
		return pgCfg, errNoConfigFound
	}
	if pgCfg.Host, ok = os.LookupEnv("POSTGRES_HOST"); !ok {
		return pgCfg, errNoConfigFound
	}
	if pgCfg.Port, ok = os.LookupEnv("POSTGRES_PORT"); !ok {
		return pgCfg, errNoConfigFound
	}
	if pgCfg.Database, ok = os.LookupEnv("POSTGRES_DATABASE"); !ok {
		return pgCfg, errNoConfigFound
	}
	if pgCfg.SSLMode, ok = os.LookupEnv("POSTGRES_SSL_MODE"); !ok {
		return pgCfg, errNoConfigFound
	}
	if poolMaxConnsStr, ok := os.LookupEnv("POSTGRES_POOL_MAX_CONNS"); !ok {
		return pgCfg, errNoConfigFound
	} else {
		pgCfg.PoolMaxConnections, err = strconv.Atoi(poolMaxConnsStr)
		if err != nil {
			return pgCfg, err
		}
	}
	if pgCfg.Password, ok = os.LookupEnv("POSTGRES_PASSWORD"); !ok {
		return pgCfg, errNoConfigFound
	}
	return pgCfg, nil
}
