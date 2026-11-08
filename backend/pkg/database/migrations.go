package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"cctv-sentinel/pkg/security"
)

const CurrentSchemaVersion = 3

// Migration represents a versioned database migration
type Migration struct {
	Version     int
	Description string
	Apply       func(s *Store) error
	Rollback    func(s *Store) error
}

// GetMigrations returns the ordered list of schema migrations
func (s *Store) GetMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "Initial schema baseline with device store and compatibility matrix",
			Apply: func(s *Store) error {
				// Base structure initialization
				if s.Devices == nil {
					s.Devices = make(map[string]modelsDevice)
				}
				if s.EncryptedCredentials == nil {
					s.EncryptedCredentials = make(map[string]string)
				}
				return nil
			},
			Rollback: func(s *Store) error {
				return nil
			},
		},
		{
			Version:     2,
			Description: "Migrate credentials to AES-256-GCM authenticated encryption",
			Apply: func(s *Store) error {
				if s.vault == nil {
					return fmt.Errorf("migration v2 requires active security vault")
				}
				if s.EncryptedCredentials == nil {
					s.EncryptedCredentials = make(map[string]string)
				}

				migratedCount := 0
				for id, dev := range s.Devices {
					// Check if device had a plaintext password in memory
					if dev.Password != "" && !s.vault.IsEncrypted(dev.Password) {
						enc, err := s.vault.Encrypt(dev.Password)
						if err != nil {
							return fmt.Errorf("failed to encrypt credential for %s: %w", id, err)
						}
						s.EncryptedCredentials[id] = enc
						dev.Password = enc
						s.Devices[id] = dev
						migratedCount++
					}
				}
				log.Printf("[MIGRATION] Schema v2 applied: encrypted %d device credentials into vault", migratedCount)
				return nil
			},
			Rollback: func(s *Store) error {
				log.Printf("[MIGRATION] Rolling back schema v2: clearing encrypted credentials table")
				s.EncryptedCredentials = make(map[string]string)
				return nil
			},
		},
		{
			Version:     3,
			Description: "Persistent desktop device registry and user accounts",
			Apply: func(s *Store) error {
				if s.DesktopDevices == nil {
					s.DesktopDevices = make(map[string]security.DesktopDeviceRecord)
				}
				if s.Users == nil {
					s.Users = make(map[string]security.UserAccount)
				}
				log.Printf("[MIGRATION] Schema v3 applied: initialized persistent desktop devices and user registries")
				return nil
			},
			Rollback: func(s *Store) error {
				log.Printf("[MIGRATION] Rolling back schema v3: clearing desktop devices and user tables")
				s.DesktopDevices = make(map[string]security.DesktopDeviceRecord)
				s.Users = make(map[string]security.UserAccount)
				return nil
			},
		},
	}
}

// RunMigrations checks the current schema version and applies all pending migrations
func (s *Store) RunMigrations() error {
	if s.SchemaVersion >= CurrentSchemaVersion {
		return nil
	}

	migrations := s.GetMigrations()
	for _, m := range migrations {
		if m.Version > s.SchemaVersion {
			log.Printf("[MIGRATION] Applying migration v%d: %s", m.Version, m.Description)

			// Create a safety backup before migration with hardened file permissions
			if s.filePath != "" {
				backupPath := fmt.Sprintf("%s.pre-v%d-%d.bak", s.filePath, m.Version, time.Now().Unix())
				if data, err := os.ReadFile(s.filePath); err == nil {
					_ = security.WriteSecureFile(backupPath, data)
				}
			}

			if err := m.Apply(s); err != nil {
				return fmt.Errorf("migration v%d failed: %w", m.Version, err)
			}
			s.SchemaVersion = m.Version
		}
	}

	return s.saveLocked()
}

// RollbackMigration rolls back migrations down to targetVersion
func (s *Store) RollbackMigration(targetVersion int) error {
	migrations := s.GetMigrations()
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if m.Version > targetVersion && m.Version <= s.SchemaVersion {
			log.Printf("[MIGRATION] Rolling back migration v%d: %s", m.Version, m.Description)
			if err := m.Rollback(s); err != nil {
				return fmt.Errorf("rollback v%d failed: %w", m.Version, err)
			}
			s.SchemaVersion = m.Version - 1
		}
	}
	return s.saveLocked()
}
