// Code generated by MockGen. DO NOT EDIT.
// Source: btcscanner/btc_scanner.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	types "github.com/babylonchain/staking-indexer/types"
	gomock "github.com/golang/mock/gomock"
)

// MockBtcScanner is a mock of BtcScanner interface.
type MockBtcScanner struct {
	ctrl     *gomock.Controller
	recorder *MockBtcScannerMockRecorder
}

// MockBtcScannerMockRecorder is the mock recorder for MockBtcScanner.
type MockBtcScannerMockRecorder struct {
	mock *MockBtcScanner
}

// NewMockBtcScanner creates a new mock instance.
func NewMockBtcScanner(ctrl *gomock.Controller) *MockBtcScanner {
	mock := &MockBtcScanner{ctrl: ctrl}
	mock.recorder = &MockBtcScannerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBtcScanner) EXPECT() *MockBtcScannerMockRecorder {
	return m.recorder
}

// ConfirmedBlocksChan mocks base method.
func (m *MockBtcScanner) ConfirmedBlocksChan() chan *types.IndexedBlock {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ConfirmedBlocksChan")
	ret0, _ := ret[0].(chan *types.IndexedBlock)
	return ret0
}

// ConfirmedBlocksChan indicates an expected call of ConfirmedBlocksChan.
func (mr *MockBtcScannerMockRecorder) ConfirmedBlocksChan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ConfirmedBlocksChan", reflect.TypeOf((*MockBtcScanner)(nil).ConfirmedBlocksChan))
}

// CurrentTipHeight mocks base method.
func (m *MockBtcScanner) CurrentTipHeight() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CurrentTipHeight")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// CurrentTipHeight indicates an expected call of CurrentTipHeight.
func (mr *MockBtcScannerMockRecorder) CurrentTipHeight() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CurrentTipHeight", reflect.TypeOf((*MockBtcScanner)(nil).CurrentTipHeight))
}

// GetRangeBlocks mocks base method.
func (m *MockBtcScanner) GetRangeBlocks(fromHeight, targetHeight uint64) ([]*types.IndexedBlock, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRangeBlocks", fromHeight, targetHeight)
	ret0, _ := ret[0].([]*types.IndexedBlock)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRangeBlocks indicates an expected call of GetRangeBlocks.
func (mr *MockBtcScannerMockRecorder) GetRangeBlocks(fromHeight, targetHeight interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRangeBlocks", reflect.TypeOf((*MockBtcScanner)(nil).GetRangeBlocks), fromHeight, targetHeight)
}

// Start mocks base method.
func (m *MockBtcScanner) Start(startHeight uint64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Start", startHeight)
	ret0, _ := ret[0].(error)
	return ret0
}

// Start indicates an expected call of Start.
func (mr *MockBtcScannerMockRecorder) Start(startHeight interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockBtcScanner)(nil).Start), startHeight)
}

// Stop mocks base method.
func (m *MockBtcScanner) Stop() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stop")
	ret0, _ := ret[0].(error)
	return ret0
}

// Stop indicates an expected call of Stop.
func (mr *MockBtcScannerMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockBtcScanner)(nil).Stop))
}
