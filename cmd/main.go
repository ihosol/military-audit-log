package main

import (
	"audit-log/internal/core"
	"audit-log/internal/db"
	"audit-log/internal/ledger"
	"audit-log/internal/storage"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

func main() {
	// --- НАЛАШТУВАННЯ ЕКСПЕРИМЕНТУ ---
	mode := flag.String("mode", "simple", "Mode: simple | bench | baseline")
	workers := flag.Int("workers", 1, "Number of concurrent threads (goroutines)")
	count := flag.Int("count", 10, "Total number of files to process")
	flag.Parse()

	fmt.Printf("🔬 Starting Experiment: Mode=%s | Workers=%d | Files=%d\n", *mode, *workers, *count)

	// 1. Ініціалізація інфраструктури
	realStorage := storage.NewMinioStorage("localhost:9000", "admin", "password123", "military-logs")
	
	// Підключаємося до Fabric (якщо не Baseline режим)
	var realLedger core.Ledger
	var err error
	
	if *mode == "baseline" {
		fmt.Println("⚠️  BASELINE MODE: Blockchain Disabled")
		realLedger = &ledger.MockLedger{} // Або nil, але краще мок, щоб не панікувало
	} else {
		realLedger, err = ledger.NewFabricLedger()
		if err != nil {
			log.Fatalf("❌ Fabric connection failed: %v", err)
		}
	}

	// БД
	realDB, err := db.NewPostgresDB("localhost", "user", "password", "audit_db", "5432")
	if err != nil {
		log.Fatalf("❌ Postgres connection failed: %v", err)
	}

	service := core.NewAuditService(realStorage, realLedger, realDB)

	// 2. Підготовка CSV
	filename := fmt.Sprintf("results_%s_w%d_c%d.csv", *mode, *workers, *count)
	file, _ := os.Create(filename)
	defer file.Close()
	writer := csv.NewWriter(file)
	writer.Write([]string{"RequestID", "Duration_Sec", "Status"})
	defer writer.Flush()

	// 3. ЗАПУСК ЕКСПЕРИМЕНТУ
	results := make(chan string, *count)
	var wg sync.WaitGroup
	
	startTime := time.Now()

	// Канал завдань (Semaphore pattern for workers)
	jobs := make(chan int, *count)

	// Запускаємо воркерів (Goroutines)
	for w := 1; w <= *workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range jobs {
				// Генерація унікального файлу
				size := 1 * 1024 * 1024 // 1 MB
				content := make([]byte, size)
				rand.Read(content[:1024]) // Випадковий заголовок (щоб хеш був різний)
				
				fName := fmt.Sprintf("req_%s_%d.bin", *mode, j)
				
				t0 := time.Now()
				
				// Виклик сервісу
				// Якщо mode == baseline, передаємо false
				useBC := (*mode != "baseline")
				_, err := service.ProcessDocument(fName, bytes.NewReader(content), int64(size), useBC)
				
				dur := time.Since(t0).Seconds()
				
				status := "OK"
				if err != nil {
					status = "ERR"
					fmt.Printf("Err: %v\n", err)
				} else {
					fmt.Printf("Worker %d: Job %d done in %.2fs\n", id, j, dur)
				}

				// Запис результату в канал (CSV рядок)
				results <- fmt.Sprintf("%d,%.4f,%s", j, dur, status)
			}
		}(w)
	}

	// Наповнюємо чергу завдань
	for j := 1; j <= *count; j++ {
		jobs <- j
	}
	close(jobs)

	// Чекаємо завершення
	wg.Wait()
	close(results)

	totalTime := time.Since(startTime)

	// Зберігаємо результати у файл
	for r := range results {
		var cols []string
		fmt.Sscanf(r, "%s", &cols) // Спрощено, краще парсити кому
		// Просто пишемо raw string у csv, розділяючи вручну для швидкості прикладу
		file.WriteString(r + "\n")
	}

	fmt.Printf("\n✅ Experiment Finished!\n")
	fmt.Printf("Total Time: %.2fs\n", totalTime.Seconds())
	fmt.Printf("Throughput (TPS): %.2f req/sec\n", float64(*count)/totalTime.Seconds())
	fmt.Printf("Data saved to: %s\n", filename)
}