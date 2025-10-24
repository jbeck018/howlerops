package turso

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Connection represents a database connection template
type Connection struct {
	ID             string                 `json:"id"`
	UserID         string                 `json:"user_id"`
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	Host           string                 `json:"host,omitempty"`
	Port           int                    `json:"port,omitempty"`
	Database       string                 `json:"database"`
	Username       string                 `json:"username,omitempty"`
	UseSSH         bool                   `json:"use_ssh,omitempty"`
	SSHHost        string                 `json:"ssh_host,omitempty"`
	SSHPort        int                    `json:"ssh_port,omitempty"`
	SSHUser        string                 `json:"ssh_user,omitempty"`
	Color          string                 `json:"color,omitempty"`
	Icon           string                 `json:"icon,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	SyncVersion    int                    `json:"sync_version"`
	OrganizationID *string                `json:"organization_id,omitempty"`
	Visibility     string                 `json:"visibility"` // 'personal' or 'shared'
	CreatedBy      string                 `json:"created_by"`
}

// ConnectionStore handles database operations for connection templates
type ConnectionStore struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewConnectionStore creates a new connection store
func NewConnectionStore(db *sql.DB, logger *logrus.Logger) *ConnectionStore {
	return &ConnectionStore{
		db:     db,
		logger: logger,
	}
}

// Create creates a new connection
func (s *ConnectionStore) Create(ctx context.Context, conn *Connection) error {
	if conn.ID == "" {
		conn.ID = uuid.New().String()
	}

	now := time.Now()
	conn.CreatedAt = now
	conn.UpdatedAt = now
	conn.SyncVersion = 1

	if conn.Visibility == "" {
		conn.Visibility = "personal"
	}

	metadataJSON, err := json.Marshal(conn.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO connection_templates (
			id, user_id, name, type, host, port, database_name, username,
			use_ssh, ssh_host, ssh_port, ssh_user, color, icon, metadata,
			created_at, updated_at, sync_version, organization_id, visibility, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		conn.ID, conn.UserID, conn.Name, conn.Type, conn.Host, conn.Port, conn.Database,
		conn.Username, conn.UseSSH, conn.SSHHost, conn.SSHPort, conn.SSHUser,
		conn.Color, conn.Icon, string(metadataJSON),
		conn.CreatedAt.Unix(), conn.UpdatedAt.Unix(), conn.SyncVersion,
		conn.OrganizationID, conn.Visibility, conn.CreatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"connection_id": conn.ID,
		"user_id":       conn.UserID,
		"name":          conn.Name,
		"visibility":    conn.Visibility,
	}).Info("Connection created")

	return nil
}

// GetByID retrieves a connection by ID
func (s *ConnectionStore) GetByID(ctx context.Context, id string) (*Connection, error) {
	query := `
		SELECT id, user_id, name, type, host, port, database_name, username,
		       use_ssh, ssh_host, ssh_port, ssh_user, color, icon, metadata,
		       created_at, updated_at, sync_version, organization_id, visibility, created_by
		FROM connection_templates
		WHERE id = ? AND deleted_at IS NULL
	`

	var conn Connection
	var metadataJSON sql.NullString
	var orgID sql.NullString
	var createdAt, updatedAt int64

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&conn.ID, &conn.UserID, &conn.Name, &conn.Type, &conn.Host, &conn.Port, &conn.Database,
		&conn.Username, &conn.UseSSH, &conn.SSHHost, &conn.SSHPort, &conn.SSHUser,
		&conn.Color, &conn.Icon, &metadataJSON,
		&createdAt, &updatedAt, &conn.SyncVersion,
		&orgID, &conn.Visibility, &conn.CreatedBy,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("connection not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query connection: %w", err)
	}

	conn.CreatedAt = time.Unix(createdAt, 0)
	conn.UpdatedAt = time.Unix(updatedAt, 0)

	if orgID.Valid {
		conn.OrganizationID = &orgID.String
	}

	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &conn.Metadata); err != nil {
			s.logger.WithError(err).Warn("Failed to unmarshal metadata")
		}
	}

	return &conn, nil
}

// GetByUserID retrieves all connections for a user (personal only)
func (s *ConnectionStore) GetByUserID(ctx context.Context, userID string) ([]*Connection, error) {
	query := `
		SELECT id, user_id, name, type, host, port, database_name, username,
		       use_ssh, ssh_host, ssh_port, ssh_user, color, icon, metadata,
		       created_at, updated_at, sync_version, organization_id, visibility, created_by
		FROM connection_templates
		WHERE user_id = ? AND visibility = 'personal' AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	return s.queryConnections(ctx, query, userID)
}

// GetConnectionsByOrganization returns all connections in an organization
// Filters by visibility: 'shared' connections visible to all org members
func (s *ConnectionStore) GetConnectionsByOrganization(ctx context.Context, orgID string) ([]*Connection, error) {
	query := `
		SELECT id, user_id, name, type, host, port, database_name, username,
		       use_ssh, ssh_host, ssh_port, ssh_user, color, icon, metadata,
		       created_at, updated_at, sync_version, organization_id, visibility, created_by
		FROM connection_templates
		WHERE organization_id = ? AND visibility = 'shared' AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	return s.queryConnections(ctx, query, orgID)
}

// GetSharedConnections returns all shared connections accessible to a user
// Includes: personal connections + shared connections in user's organizations
func (s *ConnectionStore) GetSharedConnections(ctx context.Context, userID string) ([]*Connection, error) {
	query := `
		SELECT DISTINCT ct.id, ct.user_id, ct.name, ct.type, ct.host, ct.port, ct.database_name, ct.username,
		       ct.use_ssh, ct.ssh_host, ct.ssh_port, ct.ssh_user, ct.color, ct.icon, ct.metadata,
		       ct.created_at, ct.updated_at, ct.sync_version, ct.organization_id, ct.visibility, ct.created_by
		FROM connection_templates ct
		LEFT JOIN organization_members om ON ct.organization_id = om.organization_id
		WHERE (
			(ct.user_id = ? AND ct.visibility = 'personal')
			OR (ct.visibility = 'shared' AND om.user_id = ?)
		)
		AND ct.deleted_at IS NULL
		ORDER BY ct.created_at DESC
	`

	return s.queryConnections(ctx, query, userID, userID)
}

// Update updates a connection
func (s *ConnectionStore) Update(ctx context.Context, conn *Connection) error {
	conn.UpdatedAt = time.Now()
	conn.SyncVersion++

	metadataJSON, err := json.Marshal(conn.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE connection_templates
		SET name = ?, type = ?, host = ?, port = ?, database_name = ?, username = ?,
		    use_ssh = ?, ssh_host = ?, ssh_port = ?, ssh_user = ?, color = ?, icon = ?,
		    metadata = ?, updated_at = ?, sync_version = ?, organization_id = ?, visibility = ?
		WHERE id = ? AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query,
		conn.Name, conn.Type, conn.Host, conn.Port, conn.Database, conn.Username,
		conn.UseSSH, conn.SSHHost, conn.SSHPort, conn.SSHUser, conn.Color, conn.Icon,
		string(metadataJSON), conn.UpdatedAt.Unix(), conn.SyncVersion,
		conn.OrganizationID, conn.Visibility,
		conn.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("connection not found or already deleted")
	}

	s.logger.WithField("connection_id", conn.ID).Info("Connection updated")
	return nil
}

// UpdateConnectionVisibility changes visibility (personal -> shared or vice versa)
// Only owner or admins can change visibility
func (s *ConnectionStore) UpdateConnectionVisibility(ctx context.Context, connID, userID string, visibility string) error {
	if visibility != "personal" && visibility != "shared" {
		return fmt.Errorf("invalid visibility: must be 'personal' or 'shared'")
	}

	// Get current connection to verify ownership
	conn, err := s.GetByID(ctx, connID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	// Verify user is the owner
	if conn.CreatedBy != userID {
		return fmt.Errorf("only the creator can change visibility")
	}

	query := `
		UPDATE connection_templates
		SET visibility = ?, updated_at = ?, sync_version = sync_version + 1
		WHERE id = ? AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, visibility, time.Now().Unix(), connID)
	if err != nil {
		return fmt.Errorf("failed to update visibility: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("connection not found or already deleted")
	}

	s.logger.WithFields(logrus.Fields{
		"connection_id": connID,
		"visibility":    visibility,
		"user_id":       userID,
	}).Info("Connection visibility updated")

	return nil
}

// Delete soft-deletes a connection
func (s *ConnectionStore) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE connection_templates
		SET deleted_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`

	result, err := s.db.ExecContext(ctx, query, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("connection not found or already deleted")
	}

	s.logger.WithField("connection_id", id).Info("Connection deleted")
	return nil
}

// queryConnections is a helper method to query and scan multiple connections
func (s *ConnectionStore) queryConnections(ctx context.Context, query string, args ...interface{}) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query connections: %w", err)
	}
	defer rows.Close()

	var connections []*Connection
	for rows.Next() {
		var conn Connection
		var metadataJSON sql.NullString
		var orgID sql.NullString
		var createdAt, updatedAt int64

		err := rows.Scan(
			&conn.ID, &conn.UserID, &conn.Name, &conn.Type, &conn.Host, &conn.Port, &conn.Database,
			&conn.Username, &conn.UseSSH, &conn.SSHHost, &conn.SSHPort, &conn.SSHUser,
			&conn.Color, &conn.Icon, &metadataJSON,
			&createdAt, &updatedAt, &conn.SyncVersion,
			&orgID, &conn.Visibility, &conn.CreatedBy,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}

		conn.CreatedAt = time.Unix(createdAt, 0)
		conn.UpdatedAt = time.Unix(updatedAt, 0)

		if orgID.Valid {
			conn.OrganizationID = &orgID.String
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &conn.Metadata); err != nil {
				s.logger.WithError(err).Warn("Failed to unmarshal metadata")
			}
		}

		connections = append(connections, &conn)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating connections: %w", err)
	}

	return connections, nil
}
