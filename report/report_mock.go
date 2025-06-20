// Code generated by MockGen. DO NOT EDIT.
// Source: report.go

// Package report is a generated GoMock package.
package report

import (
	context "context"
	io "io"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockReporter is a mock of Reporter interface.
type MockReporter struct {
	ctrl     *gomock.Controller
	recorder *MockReporterMockRecorder
}

// MockReporterMockRecorder is the mock recorder for MockReporter.
type MockReporterMockRecorder struct {
	mock *MockReporter
}

// NewMockReporter creates a new mock instance.
func NewMockReporter(ctrl *gomock.Controller) *MockReporter {
	mock := &MockReporter{ctrl: ctrl}
	mock.recorder = &MockReporterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReporter) EXPECT() *MockReporterMockRecorder {
	return m.recorder
}

// ReportCPUProfile mocks base method.
func (m *MockReporter) ReportCPUProfile(ctx context.Context, r io.Reader, ci CPUInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportCPUProfile", ctx, r, ci)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportCPUProfile indicates an expected call of ReportCPUProfile.
func (mr *MockReporterMockRecorder) ReportCPUProfile(ctx, r, ci interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportCPUProfile", reflect.TypeOf((*MockReporter)(nil).ReportCPUProfile), ctx, r, ci)
}

// ReportGoroutineProfile mocks base method.
func (m *MockReporter) ReportGoroutineProfile(ctx context.Context, r io.Reader, gi GoroutineInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportGoroutineProfile", ctx, r, gi)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportGoroutineProfile indicates an expected call of ReportGoroutineProfile.
func (mr *MockReporterMockRecorder) ReportGoroutineProfile(ctx, r, gi interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportGoroutineProfile", reflect.TypeOf((*MockReporter)(nil).ReportGoroutineProfile), ctx, r, gi)
}

// ReportHeapProfile mocks base method.
func (m *MockReporter) ReportHeapProfile(ctx context.Context, r io.Reader, mi MemInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportHeapProfile", ctx, r, mi)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportHeapProfile indicates an expected call of ReportHeapProfile.
func (mr *MockReporterMockRecorder) ReportHeapProfile(ctx, r, mi interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportHeapProfile", reflect.TypeOf((*MockReporter)(nil).ReportHeapProfile), ctx, r, mi)
}
