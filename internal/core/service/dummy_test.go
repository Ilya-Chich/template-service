package service

import (
	"context"
	"errors"
	"fmt"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"
	"reflect"
	"testing"
)

const updateCallPrefix = "update:"
const deleteCallPrefix = "delete:"
const testNote = "note"
const getCallPrefix = "get:"
const nameCall = "name:Example"
const insertCall = "insert"
const testName = "Example"
const invalidID = "bad"
const testID = "550e8400-e29b-41d4-a716-446655440000"

var errDependency = errors.New("dependency unavailable")

type fakeRepository struct {
	calls    []string
	get      domain.Dummy
	getErr   error
	nameErr  error
	writeErr error
	inserted domain.Dummy
	patch    domain.Patch
	ctx      context.Context
}

var _ port.DummyRepository = (*fakeRepository)(nil)

func (f *fakeRepository) Get(ctx context.Context, id string) (domain.Dummy, error) {
	f.ctx = ctx
	f.calls = append(f.calls, getCallPrefix+id)
	return f.get, f.getErr
}
func (f *fakeRepository) GetByName(ctx context.Context, name string) (domain.Dummy, error) {
	f.ctx = ctx
	f.calls = append(f.calls, "name:"+name)
	return domain.Dummy{}, f.nameErr
}
func (f *fakeRepository) Insert(ctx context.Context, dummy domain.Dummy) error {
	f.ctx = ctx
	f.calls = append(f.calls, insertCall)
	f.inserted = dummy
	return f.writeErr
}
func (f *fakeRepository) Update(ctx context.Context, id string, patch domain.Patch) error {
	f.ctx = ctx
	f.calls = append(f.calls, updateCallPrefix+id)
	f.patch = patch
	return f.writeErr
}
func (f *fakeRepository) Delete(ctx context.Context, id string) error {
	f.ctx = ctx
	f.calls = append(f.calls, deleteCallPrefix+id)
	return f.writeErr
}

type fakePublisher struct {
	calls []string
	dummy domain.Dummy
	patch domain.Patch
	err   error
	ctx   context.Context
}

var _ port.EventPublisher = (*fakePublisher)(nil)

func (f *fakePublisher) PublishCreate(ctx context.Context, dummy domain.Dummy) error {
	f.ctx, f.dummy = ctx, dummy
	f.calls = append(f.calls, "create")
	return f.err
}
func (f *fakePublisher) PublishUpdate(ctx context.Context, id string, patch domain.Patch) error {
	f.ctx, f.patch = ctx, patch
	f.calls = append(f.calls, updateCallPrefix+id)
	return f.err
}
func (f *fakePublisher) PublishDelete(ctx context.Context, id string) error {
	f.ctx = ctx
	f.calls = append(f.calls, deleteCallPrefix+id)
	return f.err
}

type fakeIDs struct {
	id    string
	err   error
	calls int
}

var _ port.IDGenerator = (*fakeIDs)(nil)

func (f *fakeIDs) NewID() (string, error) {
	f.calls++
	return f.id, f.err
}
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
			repo := &fakeRepository{}
			publisher := &fakePublisher{err: tc.publishErr}
			ids := &fakeIDs{id: tc.id, err: tc.idErr}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id, err := NewDummy(repo, publisher, ids).Create(ctx, tc.input, testNote)
			if !errors.Is(err, tc.wantErr) || ids.calls != tc.wantIDCalls {
				t.Fatalf("error=%v ID calls=%d", err, ids.calls)
			}
			if len(repo.calls) != 0 || (len(publisher.calls) != 0) != tc.wantPublished {
				t.Fatalf("repository=%v publisher=%v", repo.calls, publisher.calls)
			}
			if err == nil && id != testID || err != nil && id != "" {
				t.Fatalf("returned ID=%q error=%v", id, err)
			}
			if tc.wantPublished && (publisher.dummy != (domain.Dummy{ID: testID, Name: tc.input, Note: testNote}) || publisher.ctx != ctx) {
				t.Fatal("publication lost input or context")
			}
		})
	}
	t.Run("malformed generated ID is an internal failure", func(t *testing.T) {
		publisher := &fakePublisher{}
		_, err := NewDummy(&fakeRepository{}, publisher, &fakeIDs{id: invalidID}).Create(context.Background(), testName, "")
		if err == nil || errors.Is(err, domain.ErrInvalid) || len(publisher.calls) != 0 {
			t.Fatalf("error=%v published=%v", err, publisher.calls)
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
			repo := &fakeRepository{get: dummy, getErr: tc.repoErr}
			publisher := &fakePublisher{}
			ctx := context.Background()
			got, err := NewDummy(repo, publisher, &fakeIDs{}).Get(ctx, tc.id)
			if !errors.Is(err, tc.wantErr) || err == nil && got != dummy {
				t.Fatalf("dummy=%v error=%v", got, err)
			}
			if len(publisher.calls) != 0 || tc.id == invalidID && len(repo.calls) != 0 || tc.id != invalidID && repo.ctx != ctx {
				t.Fatal("unexpected dependency calls")
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
			repo := &fakeRepository{}
			publisher := &fakePublisher{err: tc.publishErr}
			svc := NewDummy(repo, publisher, &fakeIDs{})
			ctx := context.Background()
			var err error
			if tc.remove {
				err = svc.Delete(ctx, tc.id)
			} else {
				err = svc.Patch(ctx, tc.id, tc.patch)
			}
			if !errors.Is(err, tc.wantErr) || len(repo.calls) != 0 {
				t.Fatalf("error=%v repository=%v", err, repo.calls)
			}
			var wantCalls []string
			if tc.wantCall != "" {
				wantCalls = []string{tc.wantCall}
			}
			if !reflect.DeepEqual(publisher.calls, wantCalls) {
				t.Fatalf("publisher calls=%v", publisher.calls)
			}
			if tc.wantCall != "" && publisher.ctx != ctx {
				t.Fatal("context was not forwarded")
			}
			if !tc.remove && tc.wantCall != "" && !reflect.DeepEqual(publisher.patch, tc.patch) {
				t.Fatal("patch was not preserved")
			}
		})
	}
}
