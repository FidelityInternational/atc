// This file was generated by counterfeiter
package inputmapperfakes

import (
	"sync"

	"code.cloudfoundry.org/lager"
	"github.com/concourse/atc"
	"github.com/concourse/atc/db/algorithm"
	"github.com/concourse/atc/scheduler/inputmapper"
)

type FakeInputMapper struct {
	SaveNextInputMappingStub        func(logger lager.Logger, versions *algorithm.VersionsDB, job atc.JobConfig) (algorithm.InputMapping, error)
	saveNextInputMappingMutex       sync.RWMutex
	saveNextInputMappingArgsForCall []struct {
		logger   lager.Logger
		versions *algorithm.VersionsDB
		job      atc.JobConfig
	}
	saveNextInputMappingReturns struct {
		result1 algorithm.InputMapping
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeInputMapper) SaveNextInputMapping(logger lager.Logger, versions *algorithm.VersionsDB, job atc.JobConfig) (algorithm.InputMapping, error) {
	fake.saveNextInputMappingMutex.Lock()
	fake.saveNextInputMappingArgsForCall = append(fake.saveNextInputMappingArgsForCall, struct {
		logger   lager.Logger
		versions *algorithm.VersionsDB
		job      atc.JobConfig
	}{logger, versions, job})
	fake.recordInvocation("SaveNextInputMapping", []interface{}{logger, versions, job})
	fake.saveNextInputMappingMutex.Unlock()
	if fake.SaveNextInputMappingStub != nil {
		return fake.SaveNextInputMappingStub(logger, versions, job)
	} else {
		return fake.saveNextInputMappingReturns.result1, fake.saveNextInputMappingReturns.result2
	}
}

func (fake *FakeInputMapper) SaveNextInputMappingCallCount() int {
	fake.saveNextInputMappingMutex.RLock()
	defer fake.saveNextInputMappingMutex.RUnlock()
	return len(fake.saveNextInputMappingArgsForCall)
}

func (fake *FakeInputMapper) SaveNextInputMappingArgsForCall(i int) (lager.Logger, *algorithm.VersionsDB, atc.JobConfig) {
	fake.saveNextInputMappingMutex.RLock()
	defer fake.saveNextInputMappingMutex.RUnlock()
	return fake.saveNextInputMappingArgsForCall[i].logger, fake.saveNextInputMappingArgsForCall[i].versions, fake.saveNextInputMappingArgsForCall[i].job
}

func (fake *FakeInputMapper) SaveNextInputMappingReturns(result1 algorithm.InputMapping, result2 error) {
	fake.SaveNextInputMappingStub = nil
	fake.saveNextInputMappingReturns = struct {
		result1 algorithm.InputMapping
		result2 error
	}{result1, result2}
}

func (fake *FakeInputMapper) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.saveNextInputMappingMutex.RLock()
	defer fake.saveNextInputMappingMutex.RUnlock()
	return fake.invocations
}

func (fake *FakeInputMapper) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ inputmapper.InputMapper = new(FakeInputMapper)
