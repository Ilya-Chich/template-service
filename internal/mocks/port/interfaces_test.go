package portmocks_test

import (
	"http-api-template/internal/core/port"
	portmocks "http-api-template/internal/mocks/port"
)

var (
	_ port.AuthService     = (*portmocks.MockAuthService)(nil)
	_ port.TokenIssuer     = (*portmocks.MockTokenIssuer)(nil)
	_ port.TokenVerifier   = (*portmocks.MockTokenVerifier)(nil)
	_ port.DummyRepository = (*portmocks.MockDummyRepository)(nil)
	_ port.EventPublisher  = (*portmocks.MockEventPublisher)(nil)
	_ port.IDGenerator     = (*portmocks.MockIDGenerator)(nil)
	_ port.DummyService    = (*portmocks.MockDummyService)(nil)
	_ port.DummyProcessor  = (*portmocks.MockDummyProcessor)(nil)
	_ port.Logger          = (*portmocks.MockLogger)(nil)
)
