package container

import (
	"fmt"
	"thaily/src/server/client"
	"thaily/src/server/config"
)

// Container chứa tất cả dependencies
type Container struct {
	Config  *config.Config
	Clients *Clients
}

// Clients chứa tất cả gRPC clients và storage clients
type Clients struct {
	Academic *client.GRPCAcadamicClient
	Council  *client.GRPCCouncil
	File     *client.GRPCfile
	Role     *client.GRPCRole
	Thesis   *client.GRPCthesis
	User     *client.GRPCUser
	MinIO    *client.ServiceMinIo
	Redis    *client.RedisClient
	MongoDB  *client.MongoClient
}

// New tạo container mới và khởi tạo tất cả dependencies
func New(cfg *config.Config) (*Container, error) {
	clients, err := initClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init clients: %w", err)
	}

	return &Container{
		Config:  cfg,
		Clients: clients,
	}, nil
}

func initClients(cfg *config.Config) (*Clients, error) {

	// Initialize Redis client
	redis, err := client.NewRedisClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("redis client: %w", err)
	}

	academic, err := client.NewGRPCAcadamicClient(fmt.Sprintf("%s:%s", cfg.Services.Academic.Endpoint, cfg.Services.Academic.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("academic client: %w", err)
	}

	council, err := client.NewGRPCCouncil(fmt.Sprintf("%s:%s", cfg.Services.Council.Endpoint, cfg.Services.Council.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("council client: %w", err)
	}

	file, err := client.NewGRPCfile(fmt.Sprintf("%s:%s", cfg.Services.File.Endpoint, cfg.Services.File.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("file client: %w", err)
	}

	role, err := client.NewGRPCRole(fmt.Sprintf("%s:%s", cfg.Services.Role.Endpoint, cfg.Services.Role.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("role client: %w", err)
	}

	thesis, err := client.NewGRPCthesis(fmt.Sprintf("%s:%s", cfg.Services.Thesis.Endpoint, cfg.Services.Thesis.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("thesis client: %w", err)
	}

	user, err := client.NewGRPCUser(fmt.Sprintf("%s:%s", cfg.Services.User.Endpoint, cfg.Services.User.Port), redis.GetClient())
	if err != nil {
		return nil, fmt.Errorf("user client: %w", err)
	}

	minio, err := client.NewServiceMinIo(cfg.Services.MinIo)
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	// Initialize MongoDB client
	mongodb, err := client.NewMongoClient(cfg.MongoDB)
	if err != nil {
		return nil, fmt.Errorf("mongodb client: %w", err)
	}

	return &Clients{
		Academic: academic,
		Council:  council,
		File:     file,
		Role:     role,
		Thesis:   thesis,
		User:     user,
		MinIO:    minio,
		Redis:    redis,
		MongoDB:  mongodb,
	}, nil
}
