/*
##########################################
#            	 Yt2mpX      	   		 #
#           Made by Jettcodey            #
#                © 2025                  #
#           DO NOT REMOVE THIS           #
##########################################
*/

package main

import (
	"bufio"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DownloadStatus struct {
	Percentage string `json:"percentage"`
	Completed  bool   `json:"completed"`
	Filename   string `json:"filename,omitempty"`
	Error      string `json:"error,omitempty"`
}

type DownloadRequest struct {
	URL     string `json:"url"`
	Format  string `json:"format"`
	Quality string `json:"quality,omitempty"`
}

type DownloadEntry struct {
	Status DownloadStatus
	Mutex  sync.Mutex
}

var downloads = make(map[string]*DownloadEntry)
var downloadsMutex sync.Mutex

func generateDownloadID() string {
	return strconv.Itoa(rand.Intn(1000000))
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	log.Println("[INFO] Received download request")
	var req DownloadRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		log.Println("[ERROR] Failed to parse request payload:", err)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL cannot be empty", http.StatusBadRequest)
		log.Println("[ERROR] Empty URL received")
		return
	}

	downloadID := generateDownloadID()
	downloadsMutex.Lock()
	downloads[downloadID] = &DownloadEntry{Status: DownloadStatus{Percentage: "0%", Completed: false}}
	downloadsMutex.Unlock()

	log.Println("[INFO] Started processing for ID:", downloadID, "URL:", req.URL)

	// default to mp3
	format := strings.ToLower(req.Format)
	if format == "mp4" {
		// only allow 360p, 480p, 720p, or 1080p.
		quality := strings.TrimSpace(req.Quality)
		if quality == "" {
			quality = "1080p"
		}
		allowedQualities := map[string]bool{"360p": true, "480p": true, "720p": true, "1080p": true}
		if !allowedQualities[quality] {
			log.Println("[WARN] Invalid quality provided:", quality, "- defaulting to 1080p")
			quality = "1080p"
		}
		go startDownloadMp4(req.URL, downloadID, quality)
	} else {
		go startDownload(req.URL, downloadID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"download_id": downloadID})
}

func startDownload(url string, downloadID string) {
	outputTemplate := "downloads/%(title)s.mp3"
	log.Println("[INFO] Download ID:", downloadID, "using output template:", outputTemplate)

	// Prepare the yt-dlp command with --newline for line-by-line output.
	cmd := exec.Command("yt-dlp", "-x", "--audio-format", "mp3", "--newline", "-o", outputTemplate, url)
	// Capture combined output from stdout and stderr.
	combinedPipe, err := cmd.StdoutPipe()
	if err != nil {
		downloadsMutex.Lock()
		downloads[downloadID].Status.Error = err.Error()
		downloadsMutex.Unlock()
		log.Println("[ERROR] Failed to create stdout pipe for", downloadID, ":", err)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		downloadsMutex.Lock()
		downloads[downloadID].Status.Error = err.Error()
		downloadsMutex.Unlock()
		log.Println("[ERROR] Failed to start download for", downloadID, ":", err)
		return
	}
	log.Println("[INFO] yt-dlp process started for ID:", downloadID)

	entry := downloads[downloadID]
	scanner := bufio.NewScanner(combinedPipe)
	// Regex to extract progress percentage (e.g. "45.3%")
	progressRegex := regexp.MustCompile(`\b(\d+(?:\.\d+)?)%\b`)
	lastPercentage := ""

	// Continuously scan for progress updates.
	for scanner.Scan() {
		line := scanner.Text()
		log.Println("[DEBUG] ID:", downloadID, "Output:", line)

		// Update progress if a percentage is found.
		if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
			percentage := matches[1] + "%"
			if percentage != lastPercentage {
				log.Println("[INFO] Download ID:", downloadID, "progress:", percentage)
				lastPercentage = percentage
			}
			entry.Mutex.Lock()
			entry.Status.Percentage = percentage
			entry.Mutex.Unlock()
		}
		// get initial download destination
		if strings.HasPrefix(line, "[download] Destination: ") {
			filename := strings.TrimPrefix(line, "[download] Destination: ")
			base := filepath.Base(filename)
			log.Println("[INFO] Download ID:", downloadID, "initial destination:", base)
			entry.Mutex.Lock()
			entry.Status.Filename = base
			entry.Mutex.Unlock()
		}
		// get final converted file
		if strings.HasPrefix(line, "[ExtractAudio] Destination: ") {
			filename := strings.TrimPrefix(line, "[ExtractAudio] Destination: ")
			base := filepath.Base(filename)
			log.Println("[INFO] Download ID:", downloadID, "final destination:", base)
			entry.Mutex.Lock()
			entry.Status.Filename = base
			entry.Mutex.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		entry.Mutex.Lock()
		entry.Status.Error = err.Error()
		entry.Mutex.Unlock()
		log.Println("[ERROR] Scanner error for", downloadID, ":", err)
	}

	if err := cmd.Wait(); err != nil {
		entry.Mutex.Lock()
		entry.Status.Error = err.Error()
		entry.Mutex.Unlock()
		log.Println("[ERROR] Download failed for", downloadID, ":", err)
		return
	}

	entry.Mutex.Lock()
	entry.Status.Percentage = "100%"
	entry.Status.Completed = true
	filename := entry.Status.Filename
	entry.Mutex.Unlock()

	log.Println("[INFO] Download completed for ID:", downloadID, "File:", filename)

	// Schedule deletion of the finished file after 5 minutes.
	go func(downloadID, filename string) {
		log.Println("[INFO] Scheduling deletion for", filename, "in 5 minutes (Download ID:", downloadID, ")")
		time.Sleep(5 * time.Minute)
		path := filepath.Join("downloads", filename)
		if err := os.Remove(path); err != nil {
			log.Println("[ERROR] Failed to delete file", path, "for Download ID", downloadID, ":", err)
		} else {
			log.Println("[INFO] File", path, "deleted after 5 minutes (Download ID:", downloadID, ")")
		}
	}(downloadID, filename)
}

func startDownloadMp4(url string, downloadID string, quality string) {
	qualityNum := strings.TrimSuffix(quality, "p")
	outputTemplate := "downloads/%(title)s.mp4"
	log.Println("[INFO] Download ID:", downloadID, "using output template:", outputTemplate, "for MP4 with quality", quality)

	// specifier for selected quality.
	formatSpec := "bestvideo[height=" + qualityNum + "]+bestaudio/best"

	// conversion/merging into mp4.
	cmd := exec.Command("yt-dlp", "--merge-output-format", "mp4", "--newline", "-f", formatSpec, "-o", outputTemplate, url)
	combinedPipe, err := cmd.StdoutPipe()
	if err != nil {
		downloadsMutex.Lock()
		downloads[downloadID].Status.Error = err.Error()
		downloadsMutex.Unlock()
		log.Println("[ERROR] Failed to create stdout pipe for", downloadID, ":", err)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		downloadsMutex.Lock()
		downloads[downloadID].Status.Error = err.Error()
		downloadsMutex.Unlock()
		log.Println("[ERROR] Failed to start MP4 download for", downloadID, ":", err)
		return
	}
	log.Println("[INFO] yt-dlp MP4 process started for ID:", downloadID)

	entry := downloads[downloadID]
	scanner := bufio.NewScanner(combinedPipe)
	progressRegex := regexp.MustCompile(`\b(\d+(?:\.\d+)?)%\b`)
	lastPercentage := ""

	for scanner.Scan() {
		line := scanner.Text()
		log.Println("[DEBUG] ID:", downloadID, "Output:", line)

		if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
			percentage := matches[1] + "%"
			if percentage != lastPercentage {
				log.Println("[INFO] Download ID:", downloadID, "progress:", percentage)
				lastPercentage = percentage
			}
			entry.Mutex.Lock()
			entry.Status.Percentage = percentage
			entry.Mutex.Unlock()
		}
		if strings.HasPrefix(line, "[download] Destination: ") {
			filename := strings.TrimPrefix(line, "[download] Destination: ")
			base := filepath.Base(filename)
			log.Println("[INFO] Download ID:", downloadID, "initial destination:", base)
			entry.Mutex.Lock()
			entry.Status.Filename = base
			entry.Mutex.Unlock()
		}
		// ffmpeg output for merged MP4 files.
		if strings.HasPrefix(line, "[ffmpeg] Destination: ") {
			filename := strings.TrimPrefix(line, "[ffmpeg] Destination: ")
			base := filepath.Base(filename)
			log.Println("[INFO] Download ID:", downloadID, "final MP4 destination:", base)
			entry.Mutex.Lock()
			entry.Status.Filename = base
			entry.Mutex.Unlock()
		}
		// check to capture the final filename when merging.
		if strings.Contains(line, "Merging formats into") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && end > start {
				filename := line[start+1 : end]
				base := filepath.Base(filename)
				log.Println("[INFO] Download ID:", downloadID, "final MP4 destination (merging):", base)
				entry.Mutex.Lock()
				entry.Status.Filename = base
				entry.Mutex.Unlock()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		entry.Mutex.Lock()
		entry.Status.Error = err.Error()
		entry.Mutex.Unlock()
		log.Println("[ERROR] Scanner error for", downloadID, ":", err)
	}

	if err := cmd.Wait(); err != nil {
		entry.Mutex.Lock()
		entry.Status.Error = err.Error()
		entry.Mutex.Unlock()
		log.Println("[ERROR] MP4 download failed for", downloadID, ":", err)
		return
	}

	entry.Mutex.Lock()
	entry.Status.Percentage = "100%"
	entry.Status.Completed = true
	filename := entry.Status.Filename
	entry.Mutex.Unlock()

	log.Println("[INFO] MP4 download completed for ID:", downloadID, "File:", filename)

	// Schedule deletion of the finished file after 5 minutes.
	go func(downloadID, filename string) {
		log.Println("[INFO] Scheduling deletion for", filename, "in 5 minutes (Download ID:", downloadID, ")")
		time.Sleep(5 * time.Minute)
		path := filepath.Join("downloads", filename)
		if err := os.Remove(path); err != nil {
			log.Println("[ERROR] Failed to delete file", path, "for Download ID", downloadID, ":", err)
		} else {
			log.Println("[INFO] File", path, "deleted after 5 minutes (Download ID:", downloadID, ")")
		}
	}(downloadID, filename)
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	downloadID := r.URL.Path[len("/progress/"):]
	downloadsMutex.Lock()
	entry, exists := downloads[downloadID]
	downloadsMutex.Unlock()

	if !exists {
		http.Error(w, "Download ID not found", http.StatusNotFound)
		log.Println("[WARN] Progress request for unknown ID:", downloadID)
		return
	}

	entry.Mutex.Lock()
	json.NewEncoder(w).Encode(entry.Status)
	entry.Mutex.Unlock()
}

func handleGetFile(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Path[len("/get_file/"):]
	path := filepath.Join("downloads", filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		log.Println("[WARN] File not found:", path)
		return
	}

	log.Println("[INFO] Serving file:", path)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	http.ServeFile(w, r, path)
}

func main() {
	log.Println("[INFO] Starting Yt2mpX (v0.0.2)...")

	if _, err := os.Stat("downloads"); os.IsNotExist(err) {
		log.Println("[INFO] Creating downloads directory")
		os.Mkdir("downloads", 0755)
	}

	http.HandleFunc("/download", handleDownload)
	http.HandleFunc("/progress/", handleProgress)
	http.HandleFunc("/get_file/", handleGetFile)

	log.Println("[INFO] Server running on http://localhost:5000")
	log.Fatal(http.ListenAndServe(":5000", nil))
}
