package utils

import (
	"context"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// Service interface defining a service component.
type Service interface {
	// Init transitions a service to initialized state, blocking until done.
	Init(ctx context.Context) error
	// Start transitions an initialized service to start state, blocking until service has transitioned.
	Start(ctx context.Context) error
}

// NamedService provides its own name.
type NamedService interface {
	Name() string
}

// GracefulService supports graceful shutdown.
type GracefulService interface {
	Service

	// Stop shutdowns the service, blocking until the service has cleaned up itself.
	Stop(ctx context.Context)
}

// RunServices initializes and starts the provided services, blocking forever.
func RunServices(ctx context.Context, services ...Service) {
	// Initialize services.
	for _, service := range services {
		t := reflect.TypeOf(service)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		name := t.PkgPath() + "/" + t.Name()
		if s, ok := service.(NamedService); ok {
			name = s.Name()
		}

		log.Debugf("Initializing %s.", name)

		if err := service.Init(ctx); err != nil {
			log.Fatalf("Failed to initialize %s: %s", name, err.Error())
		}
	}

	// Start services.
	for _, service := range services {
		t := reflect.TypeOf(service)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		name := t.PkgPath() + "/" + t.Name()
		if s, ok := service.(NamedService); ok {
			name = s.Name()
		}

		log.Debugf("Starting %s.", name)

		if err := service.Start(ctx); err != nil {
			log.Fatalf("Failed to start %s: %s", name, err.Error())
		}
	}

	// Block till SIGTERM received. The default signal handler exits on SIGHUP and SIGINT without a stack dump.
	// SIGQUIT, SIGILL, SIGTRAP, SIGABRT, SIGSTKFLT, SIGEMT, and SIGSYS result in exit with a stack dump.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	<-ch

	log.Debug("SIGTERM received. Waiting for 30 seconds before closing the listeners.")
	time.Sleep(30 * time.Second)

	// Now bring down the listeners, draining them for 30 seconds.
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, service := range services {
		if s, ok := service.(GracefulService); ok {
			s.Stop(c)
		}
	}

	log.Debug("Stopped.")
}
