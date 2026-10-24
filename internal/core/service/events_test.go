package service

import (
	"context"
	"errors"
	"fmt"
	"http-api-template/internal/core/domain"
	"reflect"
	"testing"
)

func TestProcessCreate(t *testing.T) {
	notFound := fmt.Errorf("lookup: %w", domain.ErrNotFound)
	for _, tc := range []struct {
		name      string
		dummy     domain.Dummy
		getErr    error
		nameErr   error
		insertErr error
		wantErr   error
		wantCalls []string
	}{
		{insertCall, domain.Dummy{ID: testID, Name: testName}, notFound, notFound, nil, nil, []string{getCallPrefix + testID, nameCall, insertCall}},
		{"duplicate ID", domain.Dummy{ID: testID, Name: testName}, nil, notFound, nil, nil, []string{getCallPrefix + testID}},
		{"duplicate name", domain.Dummy{ID: testID, Name: testName}, notFound, nil, nil, nil, []string{getCallPrefix + testID, nameCall}},
		{"racing duplicate", domain.Dummy{ID: testID, Name: testName}, notFound, notFound, fmt.Errorf("insert: %w", domain.ErrConflict), nil, []string{getCallPrefix + testID, nameCall, insertCall}},
		{"ID lookup failure", domain.Dummy{ID: testID, Name: testName}, errDependency, nil, nil, errDependency, []string{getCallPrefix + testID}},
		{"name lookup failure", domain.Dummy{ID: testID, Name: testName}, notFound, errDependency, nil, errDependency, []string{getCallPrefix + testID, nameCall}},
		{"insert failure", domain.Dummy{ID: testID, Name: testName}, notFound, notFound, errDependency, errDependency, []string{getCallPrefix + testID, nameCall, insertCall}},
		{"invalid ID", domain.Dummy{ID: invalidID, Name: testName}, nil, nil, nil, domain.ErrInvalid, nil},
		{"invalid name", domain.Dummy{ID: testID, Name: " "}, nil, nil, nil, domain.ErrInvalid, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{getErr: tc.getErr, nameErr: tc.nameErr, writeErr: tc.insertErr}
			publisher := &fakePublisher{}
			ctx := context.Background()
			err := NewDummy(repo, publisher, &fakeIDs{}).ProcessCreate(ctx, tc.dummy)
			if !errors.Is(err, tc.wantErr) || !reflect.DeepEqual(repo.calls, tc.wantCalls) || len(publisher.calls) != 0 {
				t.Fatalf("error=%v repository=%v publisher=%v", err, repo.calls, publisher.calls)
			}
			if len(tc.wantCalls) != 0 && repo.ctx != ctx {
				t.Fatal("context was not forwarded")
			}
			if len(tc.wantCalls) == 3 && repo.inserted != tc.dummy {
				t.Fatal("insert changed the event")
			}
		})
	}
}
func TestProcessCreateReplay(t *testing.T) {
	repo := &fakeRepository{getErr: domain.ErrNotFound, nameErr: domain.ErrNotFound}
	svc := NewDummy(repo, &fakePublisher{}, &fakeIDs{})
	dummy := domain.Dummy{ID: testID, Name: testName}
	if err := svc.ProcessCreate(context.Background(), dummy); err != nil {
		t.Fatal(err)
	}
	repo.get, repo.getErr = dummy, nil
	if err := svc.ProcessCreate(context.Background(), dummy); err != nil {
		t.Fatal(err)
	}
	inserts := 0
	for _, call := range repo.calls {
		if call == insertCall {
			inserts++
		}
	}
	if inserts != 1 {
		t.Fatalf("inserts=%d", inserts)
	}
}
func TestProcessMutations(t *testing.T) {
	empty := ""
	blank := " "
	for _, tc := range []struct {
		name     string
		id       string
		patch    domain.Patch
		remove   bool
		repoErr  error
		wantErr  error
		wantCall string
	}{
		{"update", testID, domain.Patch{Note: &empty}, false, nil, nil, updateCallPrefix + testID},
		{"update missing", testID, domain.Patch{Note: &empty}, false, fmt.Errorf("update: %w", domain.ErrNotFound), nil, updateCallPrefix + testID},
		{"update failure", testID, domain.Patch{Note: &empty}, false, errDependency, errDependency, updateCallPrefix + testID},
		{"update conflict", testID, domain.Patch{Note: &empty}, false, domain.ErrConflict, domain.ErrConflict, updateCallPrefix + testID},
		{"update invalid ID", invalidID, domain.Patch{Note: &empty}, false, nil, domain.ErrInvalid, ""},
		{"update empty patch", testID, domain.Patch{}, false, nil, domain.ErrInvalid, ""},
		{"update blank name", testID, domain.Patch{Name: &blank}, false, nil, domain.ErrInvalid, ""},
		{"delete", testID, domain.Patch{}, true, nil, nil, deleteCallPrefix + testID},
		{"delete missing", testID, domain.Patch{}, true, fmt.Errorf("delete: %w", domain.ErrNotFound), nil, deleteCallPrefix + testID},
		{"delete failure", testID, domain.Patch{}, true, errDependency, errDependency, deleteCallPrefix + testID},
		{"delete invalid ID", invalidID, domain.Patch{}, true, nil, domain.ErrInvalid, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{writeErr: tc.repoErr}
			publisher := &fakePublisher{}
			svc := NewDummy(repo, publisher, &fakeIDs{})
			ctx := context.Background()
			var err error
			if tc.remove {
				err = svc.ProcessDelete(ctx, tc.id)
			} else {
				err = svc.ProcessUpdate(ctx, tc.id, tc.patch)
			}
			var wantCalls []string
			if tc.wantCall != "" {
				wantCalls = []string{tc.wantCall}
			}
			if !errors.Is(err, tc.wantErr) || !reflect.DeepEqual(repo.calls, wantCalls) || len(publisher.calls) != 0 {
				t.Fatalf("error=%v repository=%v publisher=%v", err, repo.calls, publisher.calls)
			}
			if tc.wantCall != "" && repo.ctx != ctx {
				t.Fatal("context was not forwarded")
			}
			if !tc.remove && tc.wantCall != "" && !reflect.DeepEqual(repo.patch, tc.patch) {
				t.Fatal("patch lost explicit empty note")
			}
		})
	}
}
