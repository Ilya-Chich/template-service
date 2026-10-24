package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/mock"
	"http-api-template/internal/core/domain"
	portmocks "http-api-template/internal/mocks/port"
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
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx := context.Background()
			var calls []*mock.Call
			for _, call := range tc.wantCalls {
				switch call {
				case getCallPrefix + testID:
					calls = append(calls, repo.EXPECT().Get(ctx, tc.dummy.ID).Return(domain.Dummy{}, tc.getErr).Once())
				case nameCall:
					calls = append(calls, repo.EXPECT().GetByName(ctx, tc.dummy.Name).Return(domain.Dummy{}, tc.nameErr).Once())
				case insertCall:
					calls = append(calls, repo.EXPECT().Insert(ctx, tc.dummy).Return(tc.insertErr).Once())
				default:
					t.Fatalf("unknown expected call %q", call)
				}
			}
			mock.InOrder(calls...)
			if err := NewDummy(repo, publisher, ids).ProcessCreate(ctx, tc.dummy); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestProcessCreateReplay(t *testing.T) {
	repo := portmocks.NewMockDummyRepository(t)
	svc := NewDummy(repo, portmocks.NewMockEventPublisher(t), portmocks.NewMockIDGenerator(t))
	dummy := domain.Dummy{ID: testID, Name: testName}
	ctx := context.Background()
	mock.InOrder(
		repo.EXPECT().Get(ctx, testID).Return(domain.Dummy{}, domain.ErrNotFound).Once(),
		repo.EXPECT().GetByName(ctx, testName).Return(domain.Dummy{}, domain.ErrNotFound).Once(),
		repo.EXPECT().Insert(ctx, dummy).Return(nil).Once(),
		repo.EXPECT().Get(ctx, testID).Return(dummy, nil).Once(),
	)
	for range 2 {
		if err := svc.ProcessCreate(ctx, dummy); err != nil {
			t.Fatal(err)
		}
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
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx := context.Background()
			if tc.wantCall != "" {
				if tc.remove {
					repo.EXPECT().Delete(ctx, tc.id).Return(tc.repoErr).Once()
				} else {
					repo.EXPECT().Update(ctx, tc.id, tc.patch).Return(tc.repoErr).Once()
				}
			}
			svc := NewDummy(repo, publisher, ids)
			var err error
			if tc.remove {
				err = svc.ProcessDelete(ctx, tc.id)
			} else {
				err = svc.ProcessUpdate(ctx, tc.id, tc.patch)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
