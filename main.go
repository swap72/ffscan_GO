package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ===== ANSI Colors =====
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
)

// ===== Entry Types =====
type EntryType int

const (
	TypeFile   EntryType = iota
	TypeFolder EntryType = iota
)

type Entry struct {
	Path      string
	Size      int64
	Type      EntryType
	FileCount int // only for folders
}

// ===== Human-readable size =====
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ===== Size bar =====
func sizeBar(size, maxSize int64, width int) string {
	if maxSize == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(float64(size) / float64(maxSize) * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}

// ===== Color by size =====
func sizeColor(size int64) string {
	switch {
	case size >= 1<<30: // >= 1 GB
		return colorRed
	case size >= 100<<20: // >= 100 MB
		return colorYellow
	case size >= 10<<20: // >= 10 MB
		return colorCyan
	default:
		return colorGreen
	}
}

// ===== Walk directory and collect entries =====
type Scanner struct {
	mu          sync.Mutex
	files       []Entry
	folders     []Entry
	errors      []string
	scanned     int
	showHidden  bool
	minSize     int64
	maxDepth    int
}

func (s *Scanner) scan(root string) {
	// We use a manual walk to also track folder sizes
	s.walkDir(root, root, 0)
}

func (s *Scanner) walkDir(root, dir string, depth int) int64 {
	if s.maxDepth > 0 && depth > s.maxDepth {
		return 0
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.mu.Lock()
		s.errors = append(s.errors, fmt.Sprintf("%s: %v", dir, err))
		s.mu.Unlock()
		return 0
	}

	var totalSize int64
	var fileCount int

	for _, e := range entries {
		name := e.Name()

		// Skip hidden files/folders unless flag set
		if !s.showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(dir, name)

		if e.IsDir() {
			subSize := s.walkDir(root, fullPath, depth+1)
			totalSize += subSize
			fileCount++

			if subSize >= s.minSize {
				s.mu.Lock()
				s.folders = append(s.folders, Entry{
					Path:      fullPath,
					Size:      subSize,
					Type:      TypeFolder,
					FileCount: 0, // filled below
				})
				s.mu.Unlock()
			}
		} else {
			info, err := e.Info()
			if err != nil {
				continue
			}
			sz := info.Size()
			totalSize += sz
			fileCount++

			s.mu.Lock()
			s.scanned++
			s.mu.Unlock()

			if sz >= s.minSize {
				s.mu.Lock()
				s.files = append(s.files, Entry{
					Path: fullPath,
					Size: sz,
					Type: TypeFile,
				})
				s.mu.Unlock()
			}
		}
	}

	_ = fileCount
	return totalSize
}

// ===== Print separator =====
func printSep(char string, width int, color string) {
	fmt.Printf("%s%s%s\n", color, strings.Repeat(char, width), colorReset)
}

// ===== Print banner =====
func printBanner(path string, duration time.Duration) {
	printSep("═", 70, colorBold+colorCyan)
	fmt.Printf("%s%s  🔍 DiskScanner — Large File & Folder Report%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s  Path   : %s%s%s\n", colorDim, colorBold, path, colorReset)
	fmt.Printf("%s  Scanned: %s%.2fs%s\n", colorDim, colorBold, duration.Seconds(), colorReset)
	printSep("═", 70, colorBold+colorCyan)
}

// ===== Print entries table =====
func printTable(title string, entries []Entry, topN int, icon string) {
	if len(entries) == 0 {
		fmt.Printf("\n%s%s No %s found above threshold.%s\n", colorDim, icon, strings.ToLower(title), colorReset)
		return
	}

	// Sort descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Size > entries[j].Size
	})

	if topN > 0 && len(entries) > topN {
		entries = entries[:topN]
	}

	maxSize := entries[0].Size

	fmt.Printf("\n%s%s  %s  ( %d entries )%s\n", colorBold, icon, title, len(entries), colorReset)
	printSep("─", 70, colorDim)

	fmt.Printf("%s%-4s  %-40s  %-10s  %s%s\n",
		colorBold, "#", "Path", "Size", "Usage", colorReset)
	printSep("─", 70, colorDim)

	for i, e := range entries {
		sc := sizeColor(e.Size)
		bar := sizeBar(e.Size, maxSize, 16)
		rel := e.Path

		// Trim long paths from the left
		if len(rel) > 40 {
			rel = "…" + rel[len(rel)-39:]
		}

		fmt.Printf("%s%-4d%s  %s%-40s%s  %s%-10s%s  %s%s%s\n",
			colorDim, i+1, colorReset,
			colorBold, rel, colorReset,
			sc, humanSize(e.Size), colorReset,
			sc, bar, colorReset,
		)
	}

	printSep("─", 70, colorDim)
}

// ===== Summary footer =====
func printSummary(files, folders []Entry, errors []string, minSize int64) {
	fmt.Printf("\n%s%s  Summary%s\n", colorBold, "📊", colorReset)
	printSep("─", 70, colorDim)
	fmt.Printf("  %sLarge files found   :%s %d  (>= %s)\n", colorDim, colorReset, len(files), humanSize(minSize))
	fmt.Printf("  %sLarge folders found :%s %d  (>= %s)\n", colorDim, colorReset, len(folders), humanSize(minSize))
	if len(errors) > 0 {
		fmt.Printf("  %s⚠  Skipped (no access): %d path(s)%s\n", colorYellow, len(errors), colorReset)
	}
	printSep("═", 70, colorBold+colorCyan)
	fmt.Println()
}

// ===== Spinner =====
func spinner(done <-chan struct{}, label string) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Printf("\r  %s Done!%s              \n", colorGreen, colorReset)
			return
		default:
			fmt.Printf("\r  %s%s%s %s...", colorCyan, frames[i%len(frames)], colorReset, label)
			time.Sleep(80 * time.Millisecond)
			i++
		}
	}
}

// ===== Main =====
func main() {
	// --- Flags ---
	rootPath  := flag.String("path", ".", "Root directory to scan")
	topN      := flag.Int("top", 20, "Number of top entries to show (0 = all)")
	minSizeMB := flag.Float64("min", 1.0, "Minimum size in MB to include (default: 1 MB)")
	showFiles := flag.Bool("files", true, "Show large files")
	showDirs  := flag.Bool("dirs", true, "Show large folders")
	showHidden:= flag.Bool("hidden", false, "Include hidden files/folders (dot-prefixed)")
	maxDepth  := flag.Int("depth", 0, "Max scan depth (0 = unlimited)")

	flag.Usage = func() {
		fmt.Printf("\n%s%s DiskScanner — Large File & Folder Finder%s\n\n", colorBold, colorCyan, colorReset)
		fmt.Println("  Usage: diskscanner [options]")
		fmt.Println()
		fmt.Println("  Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("  Examples:")
		fmt.Printf("    %sdiskscanner -path /home -top 10 -min 50%s\n", colorGreen, colorReset)
		fmt.Printf("    %sdiskscanner -path . -min 0.5 -dirs=false%s\n", colorGreen, colorReset)
		fmt.Printf("    %sdiskscanner -path /var/log -top 5 -hidden%s\n\n", colorGreen, colorReset)
	}

	flag.Parse()

	// Resolve absolute path
	absPath, err := filepath.Abs(*rootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Check path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%sError: path does not exist: %s%s\n", colorRed, absPath, colorReset)
		os.Exit(1)
	}

	minBytes := int64(*minSizeMB * 1024 * 1024)

	s := &Scanner{
		showHidden: *showHidden,
		minSize:    minBytes,
		maxDepth:   *maxDepth,
	}

	// Start scan with spinner
	fmt.Printf("\n%s  Scanning %s%s%s%s ...\n", colorDim, colorBold, colorCyan, absPath, colorReset)
	done := make(chan struct{})
	go spinner(done, "Scanning")

	start := time.Now()
	s.scan(absPath)
	elapsed := time.Since(start)
	close(done)
	time.Sleep(100 * time.Millisecond)

	// Print report
	printBanner(absPath, elapsed)

	if *showFiles {
		printTable("LARGEST FILES", s.files, *topN, "📄")
	}
	if *showDirs {
		printTable("LARGEST FOLDERS", s.folders, *topN, "📁")
	}

	printSummary(s.files, s.folders, s.errors, minBytes)

	// Print access errors if any
	if len(s.errors) > 0 {
		fmt.Printf("%s⚠  Access Errors:%s\n", colorYellow, colorReset)
		for _, e := range s.errors {
			fmt.Printf("  %s- %s%s\n", colorDim, e, colorReset)
		}
		fmt.Println()
	}
}
