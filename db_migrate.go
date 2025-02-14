package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	db "github.com/cometbft/cometbft-db"
)

func main() {
	dataDir := flag.String("data_dir", os.ExpandEnv(filepath.Join("$HOME", ".cometbft/data")), "CometBFT data directory")
	oldDBBackend := flag.String("old_db_backend", "goleveldb", "The current backend of the database: goleveldb, cleveldb, boltdb, rocksdb, badgerdb")
	tempDir := flag.String("temp_dir", os.ExpandEnv(filepath.Join("$HOME", ".cometbft/data_migration")), "Temporary directory to store the migrated data")

	flag.Parse()

	// 0. Create the temporary directory
	if err := os.MkdirAll(*tempDir, 0o755); err != nil {
		fatalf("Failed to create the temporary directory: %v", err)
	}

	// 1. Migrate common CometBFT databases
	cometBFTDatabases := []string{"blockstore", "state", "tx_index", "evidence"}
	for _, dbName := range cometBFTDatabases {
		fmt.Printf("Migrating %s database...\n", dbName)
		if err := migrateDB(dbName, *dataDir, *oldDBBackend, *tempDir); err != nil {
			fatalf("Failed to migrate %s database: %v. Please fix the error and restart.", dbName, err)
		}
	}

	// 2. Migrate the light client database, if it exists
	lightClientDBName := "light-client-db"
	if _, err := os.Stat(filepath.Join(*dataDir, lightClientDBName)); err == nil {
		fmt.Printf("Migrating %s database...\n", lightClientDBName)
		if err := migrateDB(lightClientDBName, *dataDir, *oldDBBackend, *tempDir); err != nil {
			fatalf("Failed to migrate %s database: %v. Please fix the error and restart.", lightClientDBName, err)
		}
	}

	// 3. Copy the migrated data to the original directory.
	confirmed := promptYesNo("Do you have a backup of the data directory?")
	if confirmed {
		fmt.Println("Copying the migrated data to the original directory...")

		entries, err := os.ReadDir(*tempDir)
		if err != nil {
			fatalf("read temp_dir: %v", err)
		}

		for _, entry := range entries {
			if entry.IsDir() { // Only process folders
				srcPath := filepath.Join(*tempDir, entry.Name())
				destPath := filepath.Join(*dataDir, entry.Name())

				// If destination folder already exists, remove it first
				if _, err := os.Stat(destPath); err == nil {
					fmt.Printf("Replacing %s\n", destPath)
					err := os.RemoveAll(destPath)
					if err != nil {
						fatalf("remove all: %v", err)
					}
				}

				err := os.Rename(srcPath, destPath)
				if err != nil {
					fatalf("rename %s: %v", entry.Name(), err)
				}

				fmt.Printf("Moved %s -> %s\n", srcPath, destPath)
			}
		}
	}

	// 4. Remove the temporary directory
	if err := os.RemoveAll(*tempDir); err != nil {
		fatalf("Failed to remove the temporary directory: %v", err)
	}

	fmt.Println("Migration completed successfully!")
}

func migrateDB(dbName, dataDir, oldDBBackend, tempDir string) error {
	oldDB, err := db.NewDB(dbName, db.BackendType(oldDBBackend), dataDir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbName, err)
	}
	defer oldDB.Close()

	newDB, err := db.NewDB(dbName, db.PebbleDBBackend, tempDir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbName, err)
	}
	defer newDB.Close()

	// Start migration
	iter, err := oldDB.Iterator(nil, nil)
	if err != nil {
		return fmt.Errorf("iterator: %w", err)
	}
	defer iter.Close()

	batch := newDB.NewBatch()

	var totalCount int

	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()
	// Channel to track the progress
	progressChan := make(chan int)
	// Goroutine to display progress
	go func() {
		for {
			select {
			case count := <-progressChan:
				fmt.Printf("\rMigrated %d records...", count)
			case <-progressTicker.C:
				fmt.Printf("\rMigrated %d records...", totalCount)
			}
		}
	}()

	startTime := time.Now()

	for ; iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		if err := batch.Set(key, value); err != nil {
			return fmt.Errorf("batch set: %w", err)
		}

		totalCount++
		if totalCount%10000 == 0 { // Send updates every 1000 records
			progressChan <- totalCount
			if err := batch.WriteSync(); err != nil {
				return fmt.Errorf("commit batch: %w", err)
			}
			if err := batch.Close(); err != nil {
				return fmt.Errorf("close batch: %w", err)
			}
			batch = newDB.NewBatch()
		}
	}

	if err := batch.WriteSync(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	if err := batch.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	// Final message
	duration := time.Since(startTime)
	fmt.Printf("\nMigration completed successfully! %d records migrated in %v\n", totalCount, duration)
	return nil
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s (y/n): ", question)

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			continue
		}

		input = strings.TrimSpace(strings.ToLower(input))

		if input == "y" || input == "yes" {
			return true
		} else if input == "n" || input == "no" {
			return false
		} else {
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	os.Exit(1)
}
