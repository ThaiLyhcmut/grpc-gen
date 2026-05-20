package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
}

func Load(envPath string) (*Config, error) {
	cfg := &Config{}

	file, err := os.Open(envPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file %s: %w", envPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "DB_HOST":
			cfg.DBHost = value
		case "DB_PORT":
			cfg.DBPort = value
		case "DB_USER":
			cfg.DBUser = value
		case "DB_PASSWORD":
			cfg.DBPassword = value
		case "DB_NAME":
			cfg.DBName = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %w", err)
	}

	// Override with environment variables if set
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.DBHost = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		cfg.DBPort = port
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.DBUser = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		cfg.DBPassword = password
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		cfg.DBName = dbName
	}

	return cfg, nil
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}
