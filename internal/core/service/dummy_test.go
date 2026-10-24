package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"http-api-template/internal/core/domain"
	portmocks "http-api-template/internal/mocks/port"

	"github.com/stretchr/testify/mock"
)

const (
	updateCallPrefix = "update:"
	deleteCallPrefix = "delete:"
	testNote         = "note"
	getCallPrefix    = "get:"
	nameCall         = "name:Example"
	insertCall       = "insert"
	testName         = "Example"
	invalidID        = "bad"
	testID           = "550e8400-e29b-41d4-a716-446655440000"
)

var errDependency = errors.New("dependency unavailable")

func TestCreate(t *testing.T) {
	for _, tc := range []struct {
		name          string
		input         string
		id            string
		idErr         error
		publishErr    error
		wantErr       error
		wantIDCalls   int
		wantPublished bool
	}{
		{"success", " Example ", testID, nil, nil, nil, 1, true},
		{"blank name", " \t", testID, nil, nil, domain.ErrInvalid, 0, false},
		{"ID failure", testName, "", errDependency, nil, errDependency, 1, false},
		{"publish failure", testName, testID, nil, errDependency, errDependency, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []*mock.Call
			if tc.wantIDCalls != 0 {
				calls = append(
					calls,
					repo.EXPECT().GetByName(ctx, tc.input).Return(domain.Dummy{}, domain.ErrNotFound).Once(),
				)
				calls = append(calls, ids.EXPECT().NewID().Return(tc.id, tc.idErr).Once())
			}
			if tc.wantPublished {
				calls = append(
					calls,
					publisher.EXPECT().
						PublishCreate(ctx, domain.Dummy{ID: testID, Name: tc.input, Note: testNote}).
						Return(tc.publishErr).
						Once(),
				)
			}
			mock.InOrder(calls...)
			id, err := NewDummy(repo, publisher, ids).Create(ctx, tc.input, testNote)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
			if err == nil && id != testID || err != nil && id != "" {
				t.Fatalf("returned ID=%q error=%v", id, err)
			}
		})
	}
	t.Run("malformed generated ID is an internal failure", func(t *testing.T) {
		repo := portmocks.NewMockDummyRepository(t)
		publisher := portmocks.NewMockEventPublisher(t)
		ids := portmocks.NewMockIDGenerator(t)
		ctx := context.Background()
		mock.InOrder(
			repo.EXPECT().GetByName(ctx, testName).Return(domain.Dummy{}, domain.ErrNotFound).Once(),
			ids.EXPECT().NewID().Return(invalidID, nil).Once(),
		)
		_, err := NewDummy(repo, publisher, ids).Create(ctx, testName, "")
		if err == nil || errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestGet(t *testing.T) {
	dummy := domain.Dummy{ID: testID, Name: testName, Note: testNote}
	for _, tc := range []struct {
		name    string
		id      string
		repoErr error
		wantErr error
	}{
		{"found", testID, nil, nil},
		{"missing", testID, fmt.Errorf("lookup: %w", domain.ErrNotFound), domain.ErrNotFound},
		{"failure", testID, errDependency, errDependency},
		{"invalid", invalidID, nil, domain.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx := context.Background()
			if tc.id != invalidID {
				repo.EXPECT().Get(ctx, tc.id).Return(dummy, tc.repoErr).Once()
			}
			got, err := NewDummy(repo, publisher, ids).Get(ctx, tc.id)
			if !errors.Is(err, tc.wantErr) || err == nil && got != dummy {
				t.Fatalf("dummy=%v error=%v", got, err)
			}
		})
	}
}

func TestWriteCommands(t *testing.T) {
	empty := ""
	blank := " "
	for _, tc := range []struct {
		name       string
		id         string
		patch      domain.Patch
		remove     bool
		publishErr error
		wantErr    error
		wantCall   string
	}{
		{"clear note", testID, domain.Patch{Note: &empty}, false, nil, nil, updateCallPrefix + testID},
		{"empty patch", testID, domain.Patch{}, false, nil, domain.ErrInvalid, ""},
		{"blank name", testID, domain.Patch{Name: &blank}, false, nil, domain.ErrInvalid, ""},
		{"bad patch ID", invalidID, domain.Patch{Note: &empty}, false, nil, domain.ErrInvalid, ""},
		{"patch publish failure", testID, domain.Patch{Note: &empty}, false, errDependency, errDependency, updateCallPrefix + testID},
		{"delete", testID, domain.Patch{}, true, nil, nil, deleteCallPrefix + testID},
		{"bad delete ID", invalidID, domain.Patch{}, true, nil, domain.ErrInvalid, ""},
		{"delete publish failure", testID, domain.Patch{}, true, errDependency, errDependency, deleteCallPrefix + testID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx := context.Background()
			if tc.wantCall != "" {
				if tc.remove {
					publisher.EXPECT().PublishDelete(ctx, tc.id).Return(tc.publishErr).Once()
				} else {
					publisher.EXPECT().PublishUpdate(ctx, tc.id, tc.patch).Return(tc.publishErr).Once()
				}
			}
			svc := NewDummy(repo, publisher, ids)
			var err error
			if tc.remove {
				err = svc.Delete(ctx, tc.id)
			} else {
				err = svc.Patch(ctx, tc.id, tc.patch)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCreateRejectsExistingNameAndLookupFailure(t *testing.T) {
	for _, tc := range []struct {
		name               string
		lookupErr, wantErr error
	}{
		{"existing name", nil, domain.ErrConflict},
		{"lookup failure", errDependency, errDependency},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := portmocks.NewMockDummyRepository(t)
			publisher := portmocks.NewMockEventPublisher(t)
			ids := portmocks.NewMockIDGenerator(t)
			ctx := context.Background()
			repo.EXPECT().GetByName(ctx, testName).Return(domain.Dummy{}, tc.lookupErr).Once()
			id, err := NewDummy(repo, publisher, ids).Create(ctx, testName, testNote)
			if id != "" || !errors.Is(err, tc.wantErr) {
				t.Fatalf("id=%q error=%v", id, err)
			}
		})
	}
}
