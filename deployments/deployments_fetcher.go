package deployments

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/cloudfoundry/bosh-cli/director"
	"github.com/prometheus/common/log"

	"github.com/cloudfoundry-community/bosh_exporter/filters"
)

type Fetcher struct {
	deploymentsFilter filters.DeploymentsFilter
}

func NewFetcher(deploymentsFilter filters.DeploymentsFilter) *Fetcher {
	return &Fetcher{deploymentsFilter: deploymentsFilter}
}

func (f *Fetcher) Deployments() ([]DeploymentInfo, error) {
	var deploymentsInfo = []DeploymentInfo{}
	var mutex = &sync.Mutex{}
	var wg = &sync.WaitGroup{}

	deployments, err := f.deploymentsFilter.GetDeployments()
	if err != nil {
		return deploymentsInfo, err
	}

	doneChannel := make(chan bool, 1)
	errChannel := make(chan error, 1)
	for _, deployment := range deployments {
		wg.Add(1)
		go func(deployment director.Deployment) {
			defer wg.Done()
			deploymentInfo, err := f.fetchDeploymentInfo(deployment)
			if err != nil {
				errChannel <- err
				return
			}

			mutex.Lock()
			deploymentsInfo = append(deploymentsInfo, *deploymentInfo)
			mutex.Unlock()
		}(deployment)
	}

	go func() {
		wg.Wait()
		close(doneChannel)
	}()

	select {
	case <-doneChannel:
	case err := <-errChannel:
		return deploymentsInfo, err
	}

	return deploymentsInfo, nil
}

func (f *Fetcher) fetchDeploymentInfo(deployment director.Deployment) (*DeploymentInfo, error) {
	deploymentInfo := &DeploymentInfo{
		Name: deployment.Name(),
	}

	errands, err := f.fetchDeploymentErrands(deployment)
	if err != nil {
		return deploymentInfo, err
	}
	deploymentInfo.Errands = errands

	instances, err := f.fetchDeploymentInstances(deployment)
	if err != nil {
		return deploymentInfo, err
	}
	deploymentInfo.Instances = instances

	releases, err := f.fetchDeploymentReleases(deployment)
	if err != nil {
		return deploymentInfo, err
	}
	deploymentInfo.Releases = releases

	stemcells, err := f.fetchDeploymentStemcells(deployment)
	if err != nil {
		return deploymentInfo, err
	}
	deploymentInfo.Stemcells = stemcells

	return deploymentInfo, nil
}

func (f *Fetcher) fetchDeploymentErrands(deployment director.Deployment) ([]Errand, error) {
	deploymentErrands := []Errand{}

	log.Debugf("Reading Errands for deployment `%s`:", deployment.Name())
	errands, err := deployment.Errands()
	if err != nil {
		return deploymentErrands, errors.New(fmt.Sprintf("Error while reading Errands for deployment `%s`: %v", deployment.Name(), err))
	}

	for _, errand := range errands {
		deploymentErrand := Errand{
			Name: errand.Name,
		}
		deploymentErrands = append(deploymentErrands, deploymentErrand)
	}

	return deploymentErrands, nil
}

func (f *Fetcher) fetchDeploymentInstances(deployment director.Deployment) ([]Instance, error) {
	deploymentInstances := []Instance{}

	log.Debugf("Reading Instances for deployment `%s`:", deployment.Name())
	instances, err := deployment.InstanceInfos()
	if err != nil {
		return deploymentInstances, errors.New(fmt.Sprintf("Error while reading Instances for deployment `%s`: %v", deployment.Name(), err))
	}

	for _, instance := range instances {
		if instance.VMID == "" {
			continue
		}

		deploymentInstance := Instance{
			AgentID:            instance.AgentID,
			Name:               instance.JobName,
			ID:                 instance.ID,
			Bootstrap:          instance.Bootstrap,
			IPs:                instance.IPs,
			AZ:                 instance.AZ,
			VMType:             instance.VMType,
			ResourcePool:       instance.ResourcePool,
			ResurrectionPaused: instance.ResurrectionPaused,
			Healthy:            instance.IsRunning(),
			Vitals: Vitals{
				CPU: CPU{
					Sys:  instance.Vitals.CPU.Sys,
					User: instance.Vitals.CPU.User,
					Wait: instance.Vitals.CPU.Wait,
				},
				Mem: Mem{
					KB:      instance.Vitals.Mem.KB,
					Percent: instance.Vitals.Mem.Percent,
				},
				Swap: Mem{
					KB:      instance.Vitals.Swap.KB,
					Percent: instance.Vitals.Swap.Percent,
				},
				Uptime: instance.Vitals.Uptime.Seconds,
				Load:   instance.Vitals.Load,
				SystemDisk: Disk{
					InodePercent: instance.Vitals.SystemDisk().InodePercent,
					Percent:      instance.Vitals.SystemDisk().Percent,
				},
				EphemeralDisk: Disk{
					InodePercent: instance.Vitals.EphemeralDisk().InodePercent,
					Percent:      instance.Vitals.EphemeralDisk().Percent,
				},
				PersistentDisk: Disk{
					InodePercent: instance.Vitals.PersistentDisk().InodePercent,
					Percent:      instance.Vitals.PersistentDisk().Percent,
				},
			},
		}

		if instance.Index != nil {
			deploymentInstance.Index = strconv.Itoa(int(*instance.Index))
		}

		deploymentProcesses := []Process{}
		for _, process := range instance.Processes {
			deploymentProcess := Process{
				Name:    process.Name,
				Uptime:  process.Uptime.Seconds,
				Healthy: process.IsRunning(),
				CPU: CPU{
					Total: process.CPU.Total,
				},
				Mem: MemInt{
					KB:      process.Mem.KB,
					Percent: process.Mem.Percent,
				},
			}
			deploymentProcesses = append(deploymentProcesses, deploymentProcess)
		}
		deploymentInstance.Processes = deploymentProcesses

		deploymentInstances = append(deploymentInstances, deploymentInstance)
	}

	return deploymentInstances, nil
}

func (f *Fetcher) fetchDeploymentReleases(deployment director.Deployment) ([]Release, error) {
	deploymentReleases := []Release{}

	log.Debugf("Reading Releases for deployment `%s`:", deployment.Name())
	releases, err := deployment.Releases()
	if err != nil {
		return deploymentReleases, errors.New(fmt.Sprintf("Error while reading Releases for deployment `%s`: %v", deployment.Name(), err))
	}

	for _, release := range releases {
		deploymentRelease := Release{
			Name:    release.Name(),
			Version: release.Version().AsString(),
		}
		deploymentReleases = append(deploymentReleases, deploymentRelease)
	}

	return deploymentReleases, nil
}

func (f *Fetcher) fetchDeploymentStemcells(deployment director.Deployment) ([]Stemcell, error) {
	deploymentStemcells := []Stemcell{}

	log.Debugf("Reading Stemcells for deployment `%s`:", deployment.Name())
	stemcells, err := deployment.Stemcells()
	if err != nil {
		return deploymentStemcells, errors.New(fmt.Sprintf("Error while reading Stemcells for deployment `%s`: %v", deployment.Name(), err))
	}

	for _, stemcell := range stemcells {
		deploymentStemcell := Stemcell{
			Name:    stemcell.Name(),
			Version: stemcell.Version().AsString(),
			OSName:  stemcell.OSName(),
		}
		deploymentStemcells = append(deploymentStemcells, deploymentStemcell)
	}

	return deploymentStemcells, nil
}
