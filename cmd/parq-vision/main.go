package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/mamorett/parq-vision/internal/collector"
	"github.com/mamorett/parq-vision/internal/config"
	"github.com/mamorett/parq-vision/internal/parquet"
	"github.com/mamorett/parq-vision/internal/vision"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to vision.json config file")
	flag.StringVar(&configPath, "c", "", "Alias for -config")
	resizeMP := flag.Float64("resize", 0, "Resize images to target Megapixels (e.g. 1.0) in-memory. 0 disables.")
	recursive := flag.Bool("recursive", false, "Scan for images recursively in subdirectories (overrides config)")
	flag.BoolVar(recursive, "r", false, "Alias for -recursive")
	batchSize := flag.Int("batch", 0, "Save progress every X images. 0 disables periodic saving.")
	flag.IntVar(batchSize, "b", 0, "Alias for -batch")
	override := flag.Bool("override", false, "Force re-processing of images already in database (default false)")
	flag.BoolVar(override, "o", false, "Alias for -override")
	stopAfter := flag.Int("stop", 0, "Stop processing after X images. 0 disables (process all).")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: parq-vision [options]\n\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  -c, -config string\n        Path to vision.json config file (required)\n")
		fmt.Fprintf(os.Stderr, "  -r, -recursive\n        Scan for images recursively (default false)\n")
		fmt.Fprintf(os.Stderr, "  -b, -batch int\n        Save progress every X images (default 0)\n")
		fmt.Fprintf(os.Stderr, "  -o, -override\n        Override idempotency; re-process and update existing entries (default false)\n")
		fmt.Fprintf(os.Stderr, "  -stop int\n        Stop processing after X images (default 0, processes all)\n")
		fmt.Fprintf(os.Stderr, "  -resize float\n        Resize images to target Megapixels (e.g. 1.0) in-memory. 0 disables. (default 0)\n")
		fmt.Fprintf(os.Stderr, "  -h, -help\n        Show this help message\n")
	}

	flag.Parse()

	if configPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 1. Collect images
	fmt.Println("Collecting images...")
	
	// Override config recursive if flag set
	finalRecursive := cfg.Images.Recursive
	if *recursive {
		finalRecursive = true
	}

	imagePaths, err := collector.CollectImages(
		cfg.Images.Source,
		finalRecursive,
		cfg.Images.Extensions,
		cfg.Images.FileList,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error collecting images: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d images\n", len(imagePaths))

	// 2. Open/Create Parquet DB
	db, err := parquet.NewDynamicParquetDB(cfg.Database.Path, cfg.Fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	// We will call db.Close() explicitly at the end or on signal

	// 3. Filter images (if not override)
	var toProcess []string
	finalOverride := cfg.Database.Override || *override
	if finalOverride {
		toProcess = imagePaths
		if *override {
			fmt.Println("Override mode enabled: all images will be re-processed.")
		}
	} else {
		skippedCount := 0
		for _, p := range imagePaths {
			if !db.Exists(p) {
				toProcess = append(toProcess, p)
			} else {
				skippedCount++
			}
		}
		if skippedCount > 0 {
			fmt.Printf("Idempotency check: skipped %d images already present in database.\n", skippedCount)
		}
	}

	if len(toProcess) == 0 {
		fmt.Println("No new images to process.")
		return
	}
	fmt.Printf("Processing %d images...\n", len(toProcess))

	// 4. Initialize Vision Client
	client := vision.NewVisionClient(cfg.LLM)

	maxPixels := 0
	if *resizeMP > 0 {
		maxPixels = int(*resizeMP * 1000000)
		fmt.Printf("In-memory resizing enabled (target: %.2f MP).\n", *resizeMP)
	}

	// 5. Processing loop
	bar := progressbar.NewOptions(len(toProcess),
		progressbar.OptionSetDescription("Processing images"),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
	
	// Signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	stopChan := make(chan struct{})
	go func() {
		<-sigChan
		fmt.Println("\nInterrupt received, saving progress and exiting...")
		close(stopChan)
	}()

	processedCount := 0
Loop:
	for _, imgPath := range toProcess {
		// Check if we've reached the stop limit
		if *stopAfter > 0 && processedCount >= *stopAfter {
			fmt.Printf("\nStop limit reached: processed %d images.\n", processedCount)
			break Loop
		}
		select {
		case <-stopChan:
			break Loop
		default:
			bar.Describe(fmt.Sprintf("Processing %s", filepath.Base(imgPath)))
			caption, err := client.DescribeImage(imgPath, cfg.Prompt, maxPixels)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError processing %s: %v\n", imgPath, err)
				bar.Add(1)
				continue
			}

			// Create row
			row := map[string]any{
				"image_path": imgPath,
			}
			for _, f := range cfg.Fields {
				switch f.Type {
				case "caption":
					row[f.FieldName] = caption
				case "timestamp":
					if f.Default == "current_timestamp" {
						row[f.FieldName] = time.Now().UTC()
					} else {
						row[f.FieldName] = nil
					}
				case "free_text", "number", "modified_at":
					row[f.FieldName] = nil
				}
			}

			if err := db.AppendRows([]map[string]any{row}, finalOverride); err != nil {
				fmt.Fprintf(os.Stderr, "\nError saving row for %s: %v\n", imgPath, err)
			}
			
			processedCount++
			if *batchSize > 0 && processedCount%*batchSize == 0 {
				if err := db.Save(); err != nil {
					fmt.Fprintf(os.Stderr, "\nError during batch save: %v\n", err)
				} else {
					fmt.Printf("\nBatch save: progress persisted to database after %d images.\n", processedCount)
				}
			}

			bar.Add(1)
		}
	}

	fmt.Println("\nSaving database...")
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error closing database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
