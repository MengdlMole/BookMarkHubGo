package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type currentFile struct {
	Version  string `json:"version"`
	Previous string `json:"previous,omitempty"`
}

type updateManifest struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	SHA256   string `json:"sha256"`
	Binary   string `json:"binary"`
}

func main() {
	root, err := installRoot()
	if err != nil {
		fatal(err)
	}
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "upgrade":
			if len(args) != 2 {
				fatal(errors.New("usage: bookmarkhub upgrade <update.zip>"))
			}
			if err := upgrade(root, args[1]); err != nil {
				fatal(err)
			}
			fmt.Println("upgrade installed successfully")
			return
		case "rollback":
			if err := rollback(root); err != nil {
				fatal(err)
			}
			fmt.Println("rollback completed")
			return
		case "version":
			current, err := readCurrent(root)
			if err != nil {
				fatal(err)
			}
			fmt.Println(current.Version)
			return
		}
	}
	if err := launch(root, args); err != nil {
		fatal(err)
	}
}

func installRoot() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}

func launch(root string, args []string) error {
	current, err := readCurrent(root)
	if err != nil {
		return err
	}
	binary := "bookmarkhub-core"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	path := filepath.Join(root, "versions", current.Version, binary)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("core %s is unavailable: %w", current.Version, err)
	}
	commandArgs := append([]string{"--home", root}, args...)
	command := exec.Command(path, commandArgs...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	return command.Run()
}

func upgrade(root, packagePath string) error {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	manifestBytes, signature, err := packageMetadata(reader.File)
	if err != nil {
		return err
	}
	if err := verifySignature(root, manifestBytes, signature); err != nil {
		return err
	}
	var manifest updateManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return err
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if manifest.Platform != platform || manifest.Arch != runtime.GOARCH {
		return fmt.Errorf("update is for %s/%s, current system is %s/%s", manifest.Platform, manifest.Arch, platform, runtime.GOARCH)
	}
	if manifest.Version == "" || strings.ContainsAny(manifest.Version, `/\\`) {
		return errors.New("invalid update version")
	}
	target := filepath.Join(root, "versions", manifest.Version)
	temp := target + ".installing"
	if err := os.RemoveAll(temp); err != nil {
		return err
	}
	if err := os.MkdirAll(temp, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	var binaryHash string
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "payload/") || file.FileInfo().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(file.Name, "payload/")
		clean := filepath.Clean(rel)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("update contains an unsafe path")
		}
		destination := filepath.Join(temp, clean)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			input.Close()
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
		closeErr := output.Close()
		input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if clean == manifest.Binary {
			binaryHash = hex.EncodeToString(hash.Sum(nil))
		}
	}
	if binaryHash == "" || !strings.EqualFold(binaryHash, manifest.SHA256) {
		return errors.New("core checksum does not match update manifest")
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.Rename(temp, target); err != nil {
		return err
	}
	current, _ := readCurrent(root)
	return writeCurrent(root, currentFile{Version: manifest.Version, Previous: current.Version})
}

func packageMetadata(files []*zip.File) ([]byte, []byte, error) {
	var manifest, signature []byte
	for _, file := range files {
		if file.Name != "manifest.json" && file.Name != "manifest.sig" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, 1<<20))
		reader.Close()
		if err != nil {
			return nil, nil, err
		}
		if file.Name == "manifest.json" {
			manifest = data
		} else {
			signature = data
		}
	}
	if len(manifest) == 0 {
		return nil, nil, errors.New("update has no manifest.json")
	}
	return manifest, signature, nil
}

func verifySignature(root string, manifest, signature []byte) error {
	keyPath := filepath.Join(root, "config", "update-public-key")
	encodedKey, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "warning: no update public key configured; relying on SHA-256 only")
		return nil
	}
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedKey)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 update public key")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || !ed25519.Verify(ed25519.PublicKey(key), manifest, sig) {
		return errors.New("update signature verification failed")
	}
	return nil
}

func rollback(root string) error {
	current, err := readCurrent(root)
	if err != nil {
		return err
	}
	if current.Previous == "" {
		return errors.New("no previous version is available")
	}
	return writeCurrent(root, currentFile{Version: current.Previous, Previous: current.Version})
}

func readCurrent(root string) (currentFile, error) {
	data, err := os.ReadFile(filepath.Join(root, "current.json"))
	if err != nil {
		return currentFile{}, err
	}
	var current currentFile
	if err := json.Unmarshal(data, &current); err != nil {
		return currentFile{}, err
	}
	if current.Version == "" || strings.ContainsAny(current.Version, `/\\`) {
		return currentFile{}, errors.New("current.json contains an invalid version")
	}
	return current, nil
}

func writeCurrent(root string, current currentFile) error {
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	temp := filepath.Join(root, "current.json.tmp")
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	target := filepath.Join(root, "current.json")
	backup := filepath.Join(root, "current.json.backup")
	_ = os.Remove(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "BookmarkHub:", err)
	os.Exit(1)
}
