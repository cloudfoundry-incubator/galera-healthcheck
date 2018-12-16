package service_manager

import (
	"io/ioutil"
	"net/http"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"

	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
)

//go:generate counterfeiter . MonitClient
type MonitClient interface {
	Start(serviceName string) error
	Stop(serviceName string) error
	Status(serviceName string) (string, error)
}

type ServiceManager struct {
	ServiceName       string
	StateFilePath     string
	MonitClient       MonitClient
	GaleraInitAddress string
	Logger            lager.Logger
}

func (m *ServiceManager) StartServiceBootstrap(_ *http.Request) (string, error) {
	if m.ServiceName == "garbd" {
		return "", errors.New("bootstrapping arbitrator not allowed")
	}

	if err := ioutil.WriteFile(m.StateFilePath, []byte("NEEDS_BOOTSTRAP"), 0777); err != nil {
		return "", errors.Wrap(err, "failed to initialize state file")
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	if err := m.waitForGaleraInit(); err != nil {
		return "", err
	}

	return "cluster bootstrap successful", nil
}

func (m *ServiceManager) StartServiceJoin(_ *http.Request) (string, error) {
	if err := ioutil.WriteFile(m.StateFilePath, []byte("CLUSTERED"), 0777); err != nil {
		return "", errors.Wrap(err, "failed to initialize state file")
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	if err := m.waitForGaleraInit(); err != nil {
		return "", err
	}

	return "join cluster successful", nil
}

func (m *ServiceManager) StartServiceSingleNode(_ *http.Request) (string, error) {
	if err := ioutil.WriteFile(m.StateFilePath, []byte("SINGLE_NODE"), 0777); err != nil {
		return "", errors.Wrap(err, "failed to initialize state file")
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	if err := m.waitForGaleraInit(); err != nil {
		return "", err
	}

	return "single node start successful", nil
}

func (m *ServiceManager) StopService(_ *http.Request) (string, error) {
	if err := m.MonitClient.Stop(m.ServiceName); err != nil {
		return "", err
	}

	return "stop successful", nil
}

func (m *ServiceManager) GetStatus(_ *http.Request) (string, error) {
	return m.MonitClient.Status(m.ServiceName)
}

func (m *ServiceManager) waitForGaleraInit() error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	httpClient := http.Client{Timeout: 1 * time.Second}

	for {
		select {
		case <-ticker.C:
			status, err := m.MonitClient.Status(m.ServiceName)
			if err != nil {
				return errors.Errorf("error fetching status for service %q", m.ServiceName)
			}

			m.Logger.Info("check-monit-state", lager.Data{
				"service": m.ServiceName,
				"state":   status,
			})

			if status != monit_client.ServiceRunning {
				return errors.New("job failed during startup")
			}

			m.Logger.Info("check-galera-init")
			res, err := httpClient.Get("http://" + m.GaleraInitAddress)
			if err != nil {
				m.Logger.Error("check-galera-init", err)
				continue
			}

			m.Logger.Info("check-galera-init", lager.Data{
				"status": res.Status,
			})

			if res.StatusCode != http.StatusOK {
				return errors.Errorf("unexpected response from node: %v", res.Status)
			}

			return nil
		}
	}
}
