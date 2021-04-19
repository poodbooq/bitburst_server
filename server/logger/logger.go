package logger

import (
	"sync"

	"go.uber.org/zap"
)

type Logger interface {
	Warn(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Error(err error, args ...interface{})
	Debug(msg string, args ...interface{})
}

type Config struct {
	IsProduction bool
}

type logger struct {
	log *zap.Logger
}

var (
	singleton *logger
	once      = new(sync.Once)
)

func Get(cfg Config) (*logger, error) {
	var err error
	once.Do(func() {
		singleton = new(logger)
		getZapFunc := zap.NewDevelopment
		if cfg.IsProduction {
			getZapFunc = zap.NewProduction
		}
		singleton.log, err = getZapFunc()
		if err != nil {
			return
		}

	})

	return singleton, err
}

func (l *logger) Close() error {
	return l.log.Sync()
}

func (l *logger) Info(msg string, args ...interface{}) {
	l.log.Sugar().Infof(msg, args...)
}
func (l *logger) Warn(msg string, args ...interface{}) {
	l.log.Sugar().Warnf(msg, args...)
}
func (l *logger) Error(err error, args ...interface{}) {
	l.log.Sugar().Errorf(err.Error(), args...)
}
func (l *logger) Debug(msg string, args ...interface{}) {
	l.log.Sugar().Debugf(msg, args...)
}
