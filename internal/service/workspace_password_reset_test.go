package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
		pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// newPasswordResetHarness builds a WorkspaceService with the mocks a reset touches. Kept in
// one place so every test below asserts the same construction; a service built differently per
// test is how one of them passes while production wires a nil field.
func newPasswordResetHarness(t *testing.T, rootEmail string) (
	*WorkspaceService,
	*mocks.MockWorkspaceRepository,
	*mocks.MockUserRepository,
	*mocks.MockAuthService,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	svc := NewWorkspaceService(
		mockRepo,
		mockUserRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mocks.NewMockUserServiceInterface(ctrl),
		mockAuthService,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: rootEmail},
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
		fullyLicensedProvider(ctrl),
	)
	return svc, mockRepo, mockUserRepo, mockAuthService
}

const (
	resetWorkspaceID = "testworkspace"
	resetOwnerID     = "owner123"
	resetTargetID    = "user123"
)

func ownerActor() (*domain.User, *domain.UserWorkspace) {
	return &domain.User{ID: resetOwnerID, Email: "owner@example.com"},
		&domain.UserWorkspace{UserID: resetOwnerID, WorkspaceID: resetWorkspaceID, Role: "owner"}
}

func TestWorkspaceService_SetUserPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("issues a password, hashes it, forces a change and drops every session", func(t *testing.T) {
		svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
			Return(&domain.User{ID: resetTargetID, Type: domain.UserTypeUser, Email: "member@example.com"}, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, resetTargetID, resetWorkspaceID).
			Return(&domain.UserWorkspace{UserID: resetTargetID, WorkspaceID: resetWorkspaceID, Role: "member"}, nil)

		var storedHash string
		mockUserRepo.EXPECT().UpdatePasswordHash(ctx, resetTargetID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, hash string) error {
				storedHash = hash
				return nil
			})
		mockUserRepo.EXPECT().SetMustChangePassword(ctx, resetTargetID, true).Return(nil)
		mockUserRepo.EXPECT().DeleteAllSessionsByUserID(ctx, resetTargetID).Return(nil)

		resp, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID,
			UserID:      resetTargetID,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// The password is shown once and is the one that was hashed: if these disagree the
		// administrator would hand over a password that cannot sign in.
		assert.Len(t, resp.Password, 20)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(resp.Password)),
			"the stored hash must verify against the password returned to the caller")

		// Stored as bcrypt, never as itself.
		assert.NotEqual(t, resp.Password, storedHash)
		assert.True(t, strings.HasPrefix(storedHash, "$2a$") || strings.HasPrefix(storedHash, "$2b$"),
			"hash %q is not bcrypt", storedHash)

		// The console is told what the server enforced rather than left to assume it.
		assert.True(t, resp.MustChangePassword)
	})

	t.Run("a non-owner member is refused", func(t *testing.T) {
		svc, _, _, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		member := &domain.User{ID: "memberactor", Email: "actor@example.com"}
		memberWorkspace := &domain.UserWorkspace{UserID: "memberactor", WorkspaceID: resetWorkspaceID, Role: "member"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, member, memberWorkspace, nil)

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
		})
		require.Error(t, err)
		var unauthorized *domain.ErrUnauthorized
		require.True(t, errors.As(err, &unauthorized), "expected ErrUnauthorized, got %T: %v", err, err)
	})

	t.Run("a target outside the workspace is refused", func(t *testing.T) {
		svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
			Return(&domain.User{ID: resetTargetID, Type: domain.UserTypeUser, Email: "elsewhere@example.com"}, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, resetTargetID, resetWorkspaceID).
			Return(nil, domain.ErrUserNotInWorkspace)

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
		})
		require.Error(t, err)
		var unauthorized *domain.ErrUnauthorized
		require.True(t, errors.As(err, &unauthorized), "expected ErrUnauthorized, got %T: %v", err, err)
	})

	t.Run("a platform administrator's password cannot be reset", func(t *testing.T) {
		svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		// The membership check passes: a root user really can be a member of a workspace.
		mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
			Return(&domain.User{ID: resetTargetID, Type: domain.UserTypeUser, Email: "root@example.com"}, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, resetTargetID, resetWorkspaceID).
			Return(&domain.UserWorkspace{UserID: resetTargetID, WorkspaceID: resetWorkspaceID, Role: "member"}, nil)

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
		})
		require.Error(t, err)
		var unauthorized *domain.ErrUnauthorized
		require.True(t, errors.As(err, &unauthorized), "expected ErrUnauthorized, got %T: %v", err, err)
	})

	t.Run("a caller cannot reset their own password through this endpoint", func(t *testing.T) {
		svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		mockUserRepo.EXPECT().GetUserByID(ctx, resetOwnerID).
			Return(&domain.User{ID: resetOwnerID, Type: domain.UserTypeUser, Email: "owner@example.com"}, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, resetOwnerID, resetWorkspaceID).
			Return(actorWorkspace, nil)

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetOwnerID,
		})
		require.Error(t, err)
		var unauthorized *domain.ErrUnauthorized
		require.True(t, errors.As(err, &unauthorized), "expected ErrUnauthorized, got %T: %v", err, err)
	})

	t.Run("an API key has no password to reset", func(t *testing.T) {
		svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
			Return(&domain.User{ID: resetTargetID, Type: domain.UserTypeAPIKey, Email: "key@example.com"}, nil)
		mockRepo.EXPECT().GetUserWorkspace(ctx, resetTargetID, resetWorkspaceID).
			Return(&domain.UserWorkspace{UserID: resetTargetID, WorkspaceID: resetWorkspaceID, Role: "member"}, nil)

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
		})
		require.Error(t, err)
	})

	t.Run("a missing target surfaces as a not-found error, not a generic failure", func(t *testing.T) {
		svc, _, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
		actor, actorWorkspace := ownerActor()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
			Return(ctx, actor, actorWorkspace, nil)
		mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		_, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
			WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
		})
		require.Error(t, err)
		var notFound *domain.ErrUserNotFound
		require.True(t, errors.As(err, &notFound), "expected ErrUserNotFound, got %T: %v", err, err)
	})
}

// The password is generated, so its properties are the only thing worth asserting: two calls
// must not agree, and no character may be one people confuse when copying it by hand.
func TestWorkspaceService_SetUserPassword_GeneratedPasswords(t *testing.T) {
	ctx := context.Background()
	svc, mockRepo, mockUserRepo, mockAuthService := newPasswordResetHarness(t, "root@example.com")
	actor, actorWorkspace := ownerActor()

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, resetWorkspaceID).
		Return(ctx, actor, actorWorkspace, nil).Times(2)
	mockUserRepo.EXPECT().GetUserByID(ctx, resetTargetID).
		Return(&domain.User{ID: resetTargetID, Type: domain.UserTypeUser, Email: "member@example.com"}, nil).Times(2)
	mockRepo.EXPECT().GetUserWorkspace(ctx, resetTargetID, resetWorkspaceID).
		Return(&domain.UserWorkspace{UserID: resetTargetID, WorkspaceID: resetWorkspaceID, Role: "member"}, nil).Times(2)
	mockUserRepo.EXPECT().UpdatePasswordHash(ctx, resetTargetID, gomock.Any()).Return(nil).Times(2)
	mockUserRepo.EXPECT().SetMustChangePassword(ctx, resetTargetID, true).Return(nil).Times(2)
	mockUserRepo.EXPECT().DeleteAllSessionsByUserID(ctx, resetTargetID).Return(nil).Times(2)

	first, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
		WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
	})
	require.NoError(t, err)
	second, err := svc.SetUserPassword(ctx, domain.SetUserPasswordRequest{
		WorkspaceID: resetWorkspaceID, UserID: resetTargetID,
	})
	require.NoError(t, err)

	assert.NotEqual(t, first.Password, second.Password, "two resets produced the same password")
	for _, pw := range []string{first.Password, second.Password} {
		assert.NotContains(t, pw, "0", "0 and O are transcription errors waiting to happen")
		assert.NotContains(t, pw, "O")
		assert.NotContains(t, pw, "1")
		assert.NotContains(t, pw, "l")
		assert.NotContains(t, pw, "I")
	}
}
