package db

import (
	"audit-log/internal/core"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgresDB struct {
	db *gorm.DB
}

// NewPostgresDB підключається до бази та робить міграцію (створює таблицю)
func NewPostgresDB(host, user, password, dbName, port string) (*PostgresDB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		host, user, password, dbName, port)

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Автоматична міграція: GORM сам створить таблицю 'documents' на основі структури
	err = database.AutoMigrate(&core.Document{})
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return &PostgresDB{db: database}, nil
}

// Save зберігає запис у таблицю
func (p *PostgresDB) Save(doc *core.Document) error {
	result := p.db.Create(doc)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// Get шукає документ за ID (PrimaryKey)
func (p *PostgresDB) Get(id string) (*core.Document, error) {
	var doc core.Document
	
	// GORM SQL: SELECT * FROM documents WHERE id = '...' LIMIT 1;
	result := p.db.First(&doc, "id = ?", id)
	
	if result.Error != nil {
		return nil, result.Error
	}
	
	return &doc, nil
}