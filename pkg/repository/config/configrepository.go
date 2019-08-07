package configrepository

import (
	"context"
	"database/sql"
	"strings"

	v1 "github.com/GameComponent/economy-service/pkg/api/v1"
	repository "github.com/GameComponent/economy-service/pkg/repository"
	"github.com/golang/protobuf/jsonpb"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"go.uber.org/zap"
)

// ConfigRepository struct
type ConfigRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewConfigRepository constructor
func NewConfigRepository(db *sql.DB, logger *zap.Logger) repository.ConfigRepository {
	return &ConfigRepository{
		db:     db,
		logger: logger,
	}
}

// Get a player
func (r *ConfigRepository) Get(ctx context.Context, key string) (*v1.Config, error) {
	var jsonString string

	err := r.db.QueryRowContext(
		ctx,
		`SELECT value FROM config WHERE key = $1`,
		key,
	).Scan(&jsonString)

	if err != nil {
		return nil, err
	}

	stringReader := strings.NewReader(jsonString)
	valueStruct := _struct.Value{}
	unmarshaler := jsonpb.Unmarshaler{}
	err = unmarshaler.Unmarshal(stringReader, &valueStruct)
	if err != nil {
		return nil, err
	}

	config := &v1.Config{
		Key:   key,
		Value: &valueStruct,
	}

	return config, nil
}

// Set a new config
func (r *ConfigRepository) Set(ctx context.Context, key string, value *_struct.Value) (*v1.Config, error) {
	marshaler := jsonpb.Marshaler{}
	jsonValue, err := marshaler.MarshalToString(value)
	if err != nil {
		return nil, err
	}

	_, err = r.db.ExecContext(
		ctx,
		`
			INSERT INTO config(key, value)
			VALUES ($1, $2)
			ON CONFLICT(key)
			DO UPDATE
			SET value = excluded.value
		`,
		key,
		jsonValue,
	)

	if err != nil {
		return nil, err
	}

	return &v1.Config{
		Key:   key,
		Value: value,
	}, nil
}

// List all configs
func (r *ConfigRepository) List(ctx context.Context, limit int32, offset int32) ([]*v1.Config, int32, error) {
	// Query configs from the database
	rows, err := r.db.QueryContext(
		ctx,
		`
			SELECT 
				key,
				value,
				(SELECT COUNT(*) FROM config) AS total_size
			FROM config
			LIMIT $1
			OFFSET $2
		`,
		limit,
		offset,
	)

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// Unwrap rows into configs
	configs := []*v1.Config{}
	totalSize := int32(0)

	for rows.Next() {
		config := v1.Config{}
		var jsonString string

		err := rows.Scan(
			&config.Key,
			&jsonString,
			&totalSize,
		)
		if err != nil {
			return nil, 0, err
		}

		stringReader := strings.NewReader(jsonString)
		unmarshaler := jsonpb.Unmarshaler{}
		valueStruct := _struct.Value{}
		err = unmarshaler.Unmarshal(stringReader, &valueStruct)
		if err != nil {
			return nil, 0, err
		}

		config.Value = &valueStruct

		configs = append(configs, &config)
	}

	return configs, totalSize, nil
}