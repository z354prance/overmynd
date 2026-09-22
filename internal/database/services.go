package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

type ServiceInput struct {
	Type       models.ServiceType
	Name       string
	Enabled    bool
	BaseURL    string
	Credential string
}

func (d *Database) CreateService(input ServiceInput) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := d.DB.Exec(`
		INSERT INTO services (
			type,
			name,
			enabled,
			base_url,
			credentials,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		string(input.Type),
		input.Name,
		input.Enabled,
		input.BaseURL,
		input.Credential,
		now,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("create service: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read service id: %w", err)
	}

	return id, nil
}

func (d *Database) ListServices() ([]models.Service, error) {
	rows, err := d.DB.Query(`
		SELECT
			id,
			type,
			name,
			enabled,
			base_url,
			credentials != '',
			created_at,
			updated_at
		FROM services
		ORDER BY type, name
	`)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []models.Service

	for rows.Next() {
		service, err := scanService(rows)
		if err != nil {
			return nil, err
		}

		services = append(services, service)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate services: %w", err)
	}

	return services, nil
}

func (d *Database) GetService(id int64) (models.Service, error) {
	row := d.DB.QueryRow(`
		SELECT
			id,
			type,
			name,
			enabled,
			base_url,
			credentials != '',
			created_at,
			updated_at
		FROM services
		WHERE id = ?
	`, id)

	service, err := scanService(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Service{}, fmt.Errorf("service not found")
		}

		return models.Service{}, err
	}

	return service, nil
}

func (d *Database) GetServiceCredential(id int64) (string, error) {
	var credential string

	err := d.DB.QueryRow(`
		SELECT credentials
		FROM services
		WHERE id = ?
	`, id).Scan(&credential)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("service not found")
		}

		return "", fmt.Errorf("get service credential: %w", err)
	}

	return credential, nil
}

func (d *Database) UpdateService(
	id int64,
	input ServiceInput,
	updateCredential bool,
) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var (
		result sql.Result
		err    error
	)

	if updateCredential {
		result, err = d.DB.Exec(`
			UPDATE services
			SET
				type = ?,
				name = ?,
				enabled = ?,
				base_url = ?,
				credentials = ?,
				updated_at = ?
			WHERE id = ?
		`,
			string(input.Type),
			input.Name,
			input.Enabled,
			input.BaseURL,
			input.Credential,
			now,
			id,
		)
	} else {
		result, err = d.DB.Exec(`
			UPDATE services
			SET
				type = ?,
				name = ?,
				enabled = ?,
				base_url = ?,
				updated_at = ?
			WHERE id = ?
		`,
			string(input.Type),
			input.Name,
			input.Enabled,
			input.BaseURL,
			now,
			id,
		)
	}

	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read update result: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("service not found")
	}

	return nil
}

func (d *Database) DeleteService(id int64) error {
	result, err := d.DB.Exec(`
		DELETE FROM services
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read delete result: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("service not found")
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanService(row scanner) (models.Service, error) {
	var (
		service     models.Service
		serviceType string
		createdAt   string
		updatedAt   string
	)

	err := row.Scan(
		&service.ID,
		&serviceType,
		&service.Name,
		&service.Enabled,
		&service.BaseURL,
		&service.HasCredential,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return models.Service{}, err
	}

	service.Type = models.ServiceType(serviceType)

	service.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return models.Service{}, fmt.Errorf(
			"parse service created time: %w",
			err,
		)
	}

	service.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return models.Service{}, fmt.Errorf(
			"parse service updated time: %w",
			err,
		)
	}

	return service, nil
}
