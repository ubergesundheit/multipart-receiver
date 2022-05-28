package main

import (
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed index.html
//go:embed assets/*
var static embed.FS

type UploadServer struct {
	targetDir string
	tmpDir    string
}

//This is where the action happens.
func (u *UploadServer) uploadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL)
	switch r.Method {
	//GET displays the upload form.
	case "GET":
		b, err := static.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)

	//POST takes the uploaded file(s) and saves it to disk.
	case "POST":
		//get the multipart reader for the request.
		reader, err := r.MultipartReader()
		if err != nil {
			log.Printf("%s %s - Error: %v", r.Method, r.URL, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//copy each part to destination.
		fileName := ""
		tmpFile, err := os.CreateTemp(u.tmpDir, "multipart-receiver")
		if err != nil {
			log.Printf("%s %s - Error: %v", r.Method, r.URL, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tmpFile.Close()

		fileHash := sha1.New()

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			fileName = strings.ToLower(part.FileName())

			//if part.FileName() is empty, skip this iteration.
			if fileName == "" {
				continue
			}

			written, err := io.Copy(tmpFile, io.TeeReader(part, fileHash))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				log.Printf("%s %s - Error: %v", r.Method, r.URL, err)
				os.Remove(tmpFile.Name())
				return
			}
			log.Printf("%s %s - Written %d bytes to %s", r.Method, r.URL, written, tmpFile.Name())
		}

		// move to final destination
		finalDestinationPath := u.generateTargetPath(fileName, hex.EncodeToString(fileHash.Sum(nil)))
		if finalDestinationPath == "" {
			err = fmt.Errorf(`{ "message": "'%s' already exists" }`, fileName)
			http.Error(w, err.Error(), http.StatusBadRequest)
			log.Printf("%s %s - Error: %v", r.Method, r.URL, err)
			os.Remove(tmpFile.Name())
			return
		}

		err = os.Rename(tmpFile.Name(), finalDestinationPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("%s %s - Error: %v", r.Method, r.URL, err)
			os.Remove(tmpFile.Name())
			return
		}

		log.Printf("%s %s - Created %s", r.Method, r.URL, finalDestinationPath)

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":"ok"}`))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (u *UploadServer) generateTargetPath(fileName, fileHash string) string {
	finalDestinationPath := path.Clean(fmt.Sprintf("%s/%s", u.targetDir, fileName))

	// if there is a file with the same name
	if fileExists(finalDestinationPath) {
		existingFileHash, err := getFileHash(finalDestinationPath)
		// check if they have a matching hash
		if err == nil && existingFileHash != fileHash {
			extension := filepath.Ext(fileName)
			fileNameWithoutExtension := strings.TrimSuffix(fileName, extension)
			// change filename
			return path.Clean(fmt.Sprintf("%s/%s_%s%s", u.targetDir, fileNameWithoutExtension, fileHash[0:7], extension))
		}

		// hashes are the same, do not accept the uploaded file
		return ""
	}

	return finalDestinationPath
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		fmt.Println(err)
		return true
	}
}

func getFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err == os.ErrNotExist {
		return "", err
	}

	defer file.Close()

	hash := sha1.New()

	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func main() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Could not determine user home dir: %v", err)
	}

	tmpDirFlag := flag.String("tmpdir", os.TempDir(), "directory for storing temporary files")
	targetDirFlag := flag.String("datadir", path.Clean(userHomeDir+"/data"), "directory for storing final files")

	log.Printf("Ensuring tmp and data directories (%s and %s)", *tmpDirFlag, *targetDirFlag)

	err = os.MkdirAll(*tmpDirFlag, 0744)
	if err != nil {
		log.Fatalf("Could not create tmp dir: %v", err)
	}
	err = os.MkdirAll(*targetDirFlag, 0744)
	if err != nil {
		log.Fatalf("Could not create data dir: %v", err)
	}

	u := UploadServer{
		tmpDir:    *tmpDirFlag,
		targetDir: *targetDirFlag,
	}

	http.HandleFunc("/upload", u.uploadHandler)

	listenAddress := "0.0.0.0:8080"

	http.Handle("/", http.FileServer(http.FS(static)))
	log.Printf("Starting server. Listening on http://%s", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
