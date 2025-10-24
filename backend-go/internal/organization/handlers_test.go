package organization

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockService is a mock implementation of the Service
type MockService struct {
	mock.Mock
}

func (m *MockService) CreateOrganization(ctx context.Context, userID string, input *CreateOrganizationInput) (*Organization, error) {
	args := m.Called(ctx, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Organization), args.Error(1)
}

func (m *MockService) GetUserOrganizations(ctx context.Context, userID string) ([]*Organization, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Organization), args.Error(1)
}

func (m *MockService) GetOrganization(ctx context.Context, orgID, userID string) (*Organization, error) {
	args := m.Called(ctx, orgID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Organization), args.Error(1)
}

func (m *MockService) UpdateOrganization(ctx context.Context, orgID, userID string, input *UpdateOrganizationInput) (*Organization, error) {
	args := m.Called(ctx, orgID, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Organization), args.Error(1)
}

func (m *MockService) DeleteOrganization(ctx context.Context, orgID, userID string) error {
	args := m.Called(ctx, orgID, userID)
	return args.Error(0)
}

func (m *MockService) GetMembers(ctx context.Context, orgID, userID string) ([]*OrganizationMember, error) {
	args := m.Called(ctx, orgID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*OrganizationMember), args.Error(1)
}

func (m *MockService) UpdateMemberRole(ctx context.Context, orgID, targetUserID, actorUserID string, role OrganizationRole) error {
	args := m.Called(ctx, orgID, targetUserID, actorUserID, role)
	return args.Error(0)
}

func (m *MockService) RemoveMember(ctx context.Context, orgID, targetUserID, actorUserID string) error {
	args := m.Called(ctx, orgID, targetUserID, actorUserID)
	return args.Error(0)
}

func (m *MockService) CreateInvitation(ctx context.Context, orgID, actorUserID string, input *CreateInvitationInput) (*OrganizationInvitation, error) {
	args := m.Called(ctx, orgID, actorUserID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OrganizationInvitation), args.Error(1)
}

func (m *MockService) GetInvitations(ctx context.Context, orgID, userID string) ([]*OrganizationInvitation, error) {
	args := m.Called(ctx, orgID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*OrganizationInvitation), args.Error(1)
}

func (m *MockService) GetPendingInvitationsForEmail(ctx context.Context, email string) ([]*OrganizationInvitation, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*OrganizationInvitation), args.Error(1)
}

func (m *MockService) AcceptInvitation(ctx context.Context, token, userID string) (*Organization, error) {
	args := m.Called(ctx, token, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Organization), args.Error(1)
}

func (m *MockService) DeclineInvitation(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockService) RevokeInvitation(ctx context.Context, orgID, invitationID, userID string) error {
	args := m.Called(ctx, orgID, invitationID, userID)
	return args.Error(0)
}

func (m *MockService) GetAuditLogs(ctx context.Context, orgID, userID string, limit, offset int) ([]*AuditLog, error) {
	args := m.Called(ctx, orgID, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*AuditLog), args.Error(1)
}

func (m *MockService) CreateAuditLog(ctx context.Context, log *AuditLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func setupHandlerTest() (*Handler, *MockService) {
	logger := logrus.New()
	logger.SetOutput(bytes.NewBuffer(nil)) // Suppress logs during tests
	mockService := new(MockService)
	handler := NewHandler(mockService, logger)
	return handler, mockService
}

func createAuthenticatedRequest(method, url string, body interface{}) *http.Request {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Add user context (simulating auth middleware)
	ctx := context.WithValue(req.Context(), "user_id", "test-user-123")
	ctx = context.WithValue(ctx, "username", "testuser")
	ctx = context.WithValue(ctx, "role", "user")

	return req.WithContext(ctx)
}

// ====================================================================
// Organization Handler Tests
// ====================================================================

func TestCreateOrganization(t *testing.T) {
	handler, mockService := setupHandlerTest()

	tests := []struct {
		name           string
		input          *CreateOrganizationInput
		mockReturn     *Organization
		mockError      error
		expectedStatus int
		checkResponse  func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name: "successful creation",
			input: &CreateOrganizationInput{
				Name:        "Test Org",
				Description: "Test Description",
			},
			mockReturn: &Organization{
				ID:          "org-123",
				Name:        "Test Org",
				Description: "Test Description",
				OwnerID:     "test-user-123",
			},
			mockError:      nil,
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var result Organization
				err := json.NewDecoder(resp.Body).Decode(&result)
				assert.NoError(t, err)
				assert.Equal(t, "org-123", result.ID)
				assert.Equal(t, "Test Org", result.Name)
			},
		},
		{
			name:           "missing name",
			input:          &CreateOrganizationInput{Description: "Test"},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockReturn != nil || tt.mockError != nil {
				mockService.On("CreateOrganization", mock.Anything, "test-user-123", tt.input).Return(tt.mockReturn, tt.mockError).Once()
				mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			req := createAuthenticatedRequest("POST", "/api/organizations", tt.input)
			resp := httptest.NewRecorder()

			handler.CreateOrganization(resp, req)

			assert.Equal(t, tt.expectedStatus, resp.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, resp)
			}

			mockService.AssertExpectations(t)
		})
	}
}

func TestListOrganizations(t *testing.T) {
	handler, mockService := setupHandlerTest()

	orgs := []*Organization{
		{ID: "org-1", Name: "Org 1"},
		{ID: "org-2", Name: "Org 2"},
	}

	mockService.On("GetUserOrganizations", mock.Anything, "test-user-123").Return(orgs, nil)

	req := createAuthenticatedRequest("GET", "/api/organizations", nil)
	resp := httptest.NewRecorder()

	handler.ListOrganizations(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), result["count"])

	mockService.AssertExpectations(t)
}

func TestGetOrganization(t *testing.T) {
	handler, mockService := setupHandlerTest()

	org := &Organization{
		ID:      "org-123",
		Name:    "Test Org",
		OwnerID: "test-user-123",
	}

	mockService.On("GetOrganization", mock.Anything, "org-123", "test-user-123").Return(org, nil)

	req := createAuthenticatedRequest("GET", "/api/organizations/org-123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.GetOrganization(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result Organization
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, "org-123", result.ID)

	mockService.AssertExpectations(t)
}

func TestUpdateOrganization(t *testing.T) {
	handler, mockService := setupHandlerTest()

	name := "Updated Org"
	input := &UpdateOrganizationInput{
		Name: &name,
	}

	org := &Organization{
		ID:   "org-123",
		Name: "Updated Org",
	}

	mockService.On("UpdateOrganization", mock.Anything, "org-123", "test-user-123", input).Return(org, nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("PUT", "/api/organizations/org-123", input)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.UpdateOrganization(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	mockService.AssertExpectations(t)
}

func TestDeleteOrganization(t *testing.T) {
	handler, mockService := setupHandlerTest()

	mockService.On("DeleteOrganization", mock.Anything, "org-123", "test-user-123").Return(nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("DELETE", "/api/organizations/org-123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.DeleteOrganization(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.True(t, result["success"].(bool))

	mockService.AssertExpectations(t)
}

// ====================================================================
// Member Handler Tests
// ====================================================================

func TestListMembers(t *testing.T) {
	handler, mockService := setupHandlerTest()

	members := []*OrganizationMember{
		{ID: "mem-1", UserID: "user-1", Role: RoleOwner},
		{ID: "mem-2", UserID: "user-2", Role: RoleMember},
	}

	mockService.On("GetMembers", mock.Anything, "org-123", "test-user-123").Return(members, nil)

	req := createAuthenticatedRequest("GET", "/api/organizations/org-123/members", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.ListMembers(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), result["count"])

	mockService.AssertExpectations(t)
}

func TestUpdateMemberRole(t *testing.T) {
	handler, mockService := setupHandlerTest()

	input := &UpdateMemberRoleInput{
		Role: RoleAdmin,
	}

	mockService.On("UpdateMemberRole", mock.Anything, "org-123", "user-456", "test-user-123", RoleAdmin).Return(nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("PUT", "/api/organizations/org-123/members/user-456", input)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123", "userId": "user-456"})
	resp := httptest.NewRecorder()

	handler.UpdateMemberRole(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	mockService.AssertExpectations(t)
}

func TestRemoveMember(t *testing.T) {
	handler, mockService := setupHandlerTest()

	mockService.On("RemoveMember", mock.Anything, "org-123", "user-456", "test-user-123").Return(nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("DELETE", "/api/organizations/org-123/members/user-456", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123", "userId": "user-456"})
	resp := httptest.NewRecorder()

	handler.RemoveMember(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	mockService.AssertExpectations(t)
}

// ====================================================================
// Invitation Handler Tests
// ====================================================================

func TestCreateInvitation(t *testing.T) {
	handler, mockService := setupHandlerTest()

	input := &CreateInvitationInput{
		Email: "test@example.com",
		Role:  RoleMember,
	}

	invitation := &OrganizationInvitation{
		ID:             "inv-123",
		OrganizationID: "org-123",
		Email:          "test@example.com",
		Role:           RoleMember,
	}

	mockService.On("CreateInvitation", mock.Anything, "org-123", "test-user-123", input).Return(invitation, nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("POST", "/api/organizations/org-123/invitations", input)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.CreateInvitation(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)

	mockService.AssertExpectations(t)
}

func TestAcceptInvitation(t *testing.T) {
	handler, mockService := setupHandlerTest()

	org := &Organization{
		ID:   "org-123",
		Name: "Test Org",
	}

	mockService.On("AcceptInvitation", mock.Anything, "inv-token-123", "test-user-123").Return(org, nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil)

	req := createAuthenticatedRequest("POST", "/api/invitations/inv-token-123/accept", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "inv-token-123"})
	resp := httptest.NewRecorder()

	handler.AcceptInvitation(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.True(t, result["success"].(bool))

	mockService.AssertExpectations(t)
}

func TestDeclineInvitation(t *testing.T) {
	handler, mockService := setupHandlerTest()

	mockService.On("DeclineInvitation", mock.Anything, "inv-token-123").Return(nil)
	mockService.On("CreateAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()

	req := createAuthenticatedRequest("POST", "/api/invitations/inv-token-123/decline", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "inv-token-123"})
	resp := httptest.NewRecorder()

	handler.DeclineInvitation(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	mockService.AssertExpectations(t)
}

// ====================================================================
// Audit Log Handler Tests
// ====================================================================

func TestGetAuditLogs(t *testing.T) {
	handler, mockService := setupHandlerTest()

	logs := []*AuditLog{
		{ID: "log-1", Action: "organization.created"},
		{ID: "log-2", Action: "member.added"},
	}

	mockService.On("GetAuditLogs", mock.Anything, "org-123", "test-user-123", 50, 0).Return(logs, nil)

	req := createAuthenticatedRequest("GET", "/api/organizations/org-123/audit-logs", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "org-123"})
	resp := httptest.NewRecorder()

	handler.GetAuditLogs(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), result["count"])

	mockService.AssertExpectations(t)
}

// ====================================================================
// Helper Function Tests
// ====================================================================

func TestExtractIPAddress(t *testing.T) {
	handler, _ := setupHandlerTest()

	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-Forwarded-For header",
			headers:  map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.1"},
			expected: "192.168.1.1",
		},
		{
			name:     "X-Real-IP header",
			headers:  map[string]string{"X-Real-IP": "192.168.1.2"},
			expected: "192.168.1.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := handler.extractIPAddress(req)
			assert.Equal(t, tt.expected, ip)
		})
	}
}
