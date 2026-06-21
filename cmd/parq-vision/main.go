package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mamorett/parq-vision/internal/collector"
	"github.com/mamorett/parq-vision/internal/config"
	"github.com/mamorett/parq-vision/internal/parquet"
	"github.com/mamorett/parq-vision/internal/progress"
	"github.com/mamorett/parq-vision/internal/vision"
)

type result struct {
	imagePath string
	caption   string
	err       error
}

var rawAsciiArt = `  ____   _    ____   ___   __     _____ ____ ___ ___  _   _ 
 |  _ \ / \  |  _ \ / _ \  \ \   / /_ _/ ___|_ _/ _ \| \ | |
 | |_) / _ \ | |_) | | | |  \ \ / / | |\___ \| | | | |  \| |
 |  __/ ___ \|  _ <| |_| |   \ V /  | | ___) | | |_| | |\  |
 |_| /_/   \_\_| \_\\__\_\    \_/  |___|____/___\___/|_| \_|`

func printLogo() {
	fmt.Printf("\033[36m%s\033[0m\n\n", rawAsciiArt)
}

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
	concurrency := flag.Int("concurrency", 0, "Number of parallel LLM workers (overrides config, default from config or 1)")
	flag.IntVar(concurrency, "j", 0, "Alias for -concurrency")
	inspectPath := flag.String("inspect", "", "Path to a Parquet database file to inspect natively")

	flag.Usage = func() {
		printLogo()
		fmt.Fprintf(os.Stderr, "\033[1;36mUsage of parq-vision:\033[0m\n\n")
		fmt.Fprintf(os.Stderr, "\033[1mOptions:\033[0m\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-c, --config\033[0m \033[33m<path>\033[0m        Path to vision.json config file (\033[1;31mrequired\033[0m)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-j, --concurrency\033[0m \033[33m<int>\033[0m     Number of parallel LLM workers (overrides config)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-r, --recursive\033[0m             Scan for images recursively (default false)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-b, --batch\033[0m \033[33m<int>\033[0m           Save progress every X images (default 0, disables periodic saving)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-o, --override\033[0m              Force re-processing of images already in database\n")
		fmt.Fprintf(os.Stderr, "  \033[36m--stop\033[0m \033[33m<int>\033[0m                Stop processing after X images (default 0)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m--resize\033[0m \033[33m<float>\033[0m            Resize images to target Megapixels in-memory (0 disables)\n")
		fmt.Fprintf(os.Stderr, "  \033[36m--inspect\033[0m \033[33m<path>\033[0m           Path to a Parquet database file to inspect natively\n")
		fmt.Fprintf(os.Stderr, "  \033[36m-h, --help\033[0m                  Show this help message\n\n")
		fmt.Fprintf(os.Stderr, "\033[1mExamples:\033[0m\n")
		fmt.Fprintf(os.Stderr, "  \033[90mparq-vision -c vision.json\033[0m\n")
		fmt.Fprintf(os.Stderr, "  \033[90mparq-vision -c vision.json -j 4 --override\033[0m\n")
		fmt.Fprintf(os.Stderr, "  \033[90mparq-vision --inspect output.parquet\033[0m\n")
	}

	flag.Parse()

	if *inspectPath != "" {
		if err := parquet.Inspect(*inspectPath); err != nil {
			fmt.Printf("\033[31m✗ Error inspecting database:\033[0m %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if configPath == "" {
		fmt.Println("\033[31m✗ Error: -config is required\033[0m")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("\033[31m✗ Error loading config:\033[0m %v\n", err)
		os.Exit(1)
	}

	finalConcurrency := cfg.LLM.Concurrency
	if *concurrency > 0 {
		finalConcurrency = *concurrency
	}
	if finalConcurrency < 1 {
		finalConcurrency = 1
	}

	// 1. Collect images
	fmt.Println("Collecting images...")

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
		fmt.Printf("\033[31m✗ Error collecting images:\033[0m %v\n", err)
		os.Exit(1)
	}

	// 2. Open/Create Parquet DB
	db, err := parquet.NewDynamicParquetDB(cfg.Database.Path, cfg.Fields)
	if err != nil {
		fmt.Printf("\033[31m✗ Error opening database:\033[0m %v\n", err)
		os.Exit(1)
	}

	// 3. Filter images (if not override)
	var toProcess []string
	var skippedCount int
	finalOverride := cfg.Database.Override || *override
	if finalOverride {
		toProcess = imagePaths
	} else {
		for _, p := range imagePaths {
			if !db.Exists(p) {
				toProcess = append(toProcess, p)
			} else {
				skippedCount++
			}
		}
	}

	if *stopAfter > 0 && *stopAfter < len(toProcess) {
		toProcess = toProcess[:*stopAfter]
	}

	// 4. Initialize Vision Client
	client := vision.NewVisionClient(cfg.LLM)

	maxPixels := 0
	if *resizeMP > 0 {
		maxPixels = int(*resizeMP * 1000000)
	}

	// Print visual header
	printLogo()
	fmt.Printf("\033[1;36m🎨 parq-vision (Go)\033[0m\n")
	fmt.Printf("\033[90mConfig:\033[0m %s\n", configPath)
	fmt.Printf("\033[90mConcurrency:\033[0m %d\n", finalConcurrency)
	fmt.Printf("\033[90mRecursive:\033[0m %v\n", finalRecursive)
	fmt.Printf("\033[90mBatch size:\033[0m %d\n", *batchSize)
	fmt.Printf("\033[90mOverride enabled:\033[0m %v\n", finalOverride)
	if *stopAfter > 0 {
		fmt.Printf("\033[90mStop after:\033[0m %d images\n", *stopAfter)
	}
	if *resizeMP > 0 {
		fmt.Printf("\033[90mIn-memory resizing:\033[0m %.2f MP\n", *resizeMP)
	}
	fmt.Println("\n💡 \033[33mTip:\033[0m Press \033[1;33mCtrl-C\033[0m anytime to save progress and exit gracefully")
	fmt.Println("\033[90m" + strings.Repeat("-", 60) + "\033[0m")

	fmt.Printf("Found %d image(s) total\n", len(imagePaths))
	if skippedCount > 0 {
		fmt.Printf("Skipping %d image(s) already in database\n", skippedCount)
	}

	if len(toProcess) == 0 {
		fmt.Println("\n✓ No images to process. All images already exist in database.")
		db.Close()
		return
	}

	fmt.Printf("Processing %d image(s)\n\n", len(toProcess))

	// 5. Progress bar
	bar := progress.NewProgressBar(len(toProcess), "Processing images")

	// 6. Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var successCount int64
	var errorCount int64

	stopChan := make(chan struct{})
	go func() {
		<-sigChan
		fmt.Println("\n\n⚠ Interrupt received (Ctrl-C). Saving progress...")
		close(stopChan)
	}()

	// 7. Concurrent worker pool
	jobs := make(chan string, finalConcurrency*2)
	results := make(chan result, finalConcurrency*2)

	var wg sync.WaitGroup
	for w := 0; w < finalConcurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for imgPath := range jobs {
				select {
				case <-stopChan:
					return
				default:
				}

				baseName := filepath.Base(imgPath)
				bar.Describe("Processing " + baseName)

				caption, err := client.DescribeImage(imgPath, cfg.Prompt, maxPixels)
				results <- result{imagePath: imgPath, caption: caption, err: err}
			}
		}()
	}

	// Feed jobs
	go func() {
		for _, imgPath := range toProcess {
			select {
			case <-stopChan:
				break
			default:
			}
			select {
			case jobs <- imgPath:
			case <-stopChan:
			}
		}
		close(jobs)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		baseName := filepath.Base(res.imagePath)
		if res.err != nil {
			atomic.AddInt64(&errorCount, 1)
			bar.UpdateWithStatus(fmt.Sprintf("✗ %s: %v", baseName, res.err))
			continue
		}

		row := map[string]any{
			"image_path": res.imagePath,
		}
		for _, f := range cfg.Fields {
			switch f.Type {
			case "caption":
				row[f.FieldName] = res.caption
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
			atomic.AddInt64(&errorCount, 1)
			bar.UpdateWithStatus(fmt.Sprintf("✗ Error saving row for %s: %v", baseName, err))
			continue
		}

		count := int(atomic.AddInt64(&successCount, 1))
		bar.IncrementWithStatus(fmt.Sprintf("✓ %s", baseName))

		if *batchSize > 0 && count%*batchSize == 0 {
			if err := db.Save(); err != nil {
				bar.UpdateWithStatus(fmt.Sprintf("✗ Error during batch save: %v", err))
			} else {
				bar.UpdateWithStatus(fmt.Sprintf("✓ Batch save: progress persisted after %d images", count))
			}
		}
	}

	bar.Finish()

	fmt.Println("Saving results to database...")
	if err := db.Close(); err != nil {
		fmt.Printf("\033[31m✗ Error closing database:\033[0m %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("\033[32m✓ Database updated:\033[0m %s\n", cfg.Database.Path)
	}

	fmt.Println("\033[90m" + strings.Repeat("-", 60) + "\033[0m")
	fmt.Println("\033[1;36mProcessing complete!\033[0m")
	fmt.Printf("\033[32m✓ Successfully processed:\033[0m %d\n", successCount)
	if errorCount > 0 {
		fmt.Printf("\033[31m✗ Errors:\033[0m %d\n", errorCount)
	}
	if skippedCount > 0 {
		fmt.Printf("\033[90m⊘ Skipped (already in database):\033[0m %d\n", skippedCount)
	}
}
