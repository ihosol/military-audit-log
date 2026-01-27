package main

import (
	"audit-log/internal/core"
	"audit-log/internal/ledger"
	"audit-log/internal/storage"
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"
)

// DummyDB (залишаємо як є)
type DummyDB struct{}
func (d *DummyDB) Save(doc *core.Document) error { return nil }

func main() {
	fmt.Println("🚀 Starting Benchmark for Research Paper...")

	// 1. Підключення
	realStorage := storage.NewMinioStorage("localhost:9000", "admin", "password123", "military-logs")
	realLedger, err := ledger.NewFabricLedger()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Fabric: %v", err)
	}
	defer realLedger.Close()

	myDB := &DummyDB{}
	service := core.NewAuditService(realStorage, realLedger, myDB)

	// 2. Підготовка CSV файлу
	file, err := os.Create("benchmark_results.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Заголовки CSV
	writer.Write([]string{"Iteration", "FileSize_Bytes", "Latency_Seconds"})

	// 3. ПАРАМЕТРИ ТЕСТУ
	iterations := 10        // Скільки разів запускати (для статті постав 50-100)
	fileSize := 1024 * 1024 // 1 МБ (можеш змінювати на 10 МБ)

	fmt.Printf("Running %d iterations with %d byte files...\n", iterations, fileSize)

	// 4. Цикл тестування
	for i := 1; i <= iterations; i++ {
		// Генерація даних: створюємо масив нулів
		content := make([]byte, fileSize)
		
		// !!! ВАЖЛИВА ЗМІНА !!!
		// Додаємо унікальні дані на початок файлу, щоб хеш завжди був різним
		uniquePrefix := fmt.Sprintf("Iteration-%d-Time-%d", i, time.Now().UnixNano())
		copy(content, []byte(uniquePrefix)) // Копіюємо унікальний рядок у початок масиву

		reader := bytes.NewReader(content)
		fileName := fmt.Sprintf("bench_file_%d.bin", i)

		fmt.Printf("[%d/%d] Processing... ", i, iterations)
		
		start := time.Now()
		
		_, err := service.ProcessDocument(fileName, reader, int64(fileSize))
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}

		duration := time.Since(start).Seconds()
		fmt.Printf("Done in %.4fs\n", duration)

		// Запис у CSV
		writer.Write([]string{
			fmt.Sprintf("%d", i),
			fmt.Sprintf("%d", fileSize),
			fmt.Sprintf("%.4f", duration),
		})
	}

	fmt.Println("✅ Benchmark finished! Data saved to 'benchmark_results.csv'")
}