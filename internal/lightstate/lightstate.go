package lightstate

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/nex-crm/wuphf/internal/backup"
	"github.com/nex-crm/wuphf/internal/config"
)

const (
	companyObjectKey         = "state/company.json"
	onboardedObjectKey       = "state/onboarded.json"
	bootstrapObjectKey       = "state/cloud-backup-bootstrap.json"
	codexAuthObjectKey       = "state/codex/auth.json"
	codexConfigObjectKey     = "state/codex/config.toml"
	codexSkillsObjectKey     = "state/codex/skills.zip"
	agentsSkillsObjectKey    = "state/agents/skills.zip"
	gcloudADCObjectKey       = "state/gcloud/application_default_credentials.json"
	defaultJSONContentType   = "application/json"
	defaultTOMLContentType   = "application/toml"
	defaultZipContentType    = "application/zip"
	defaultBinaryContentType = "application/octet-stream"
)

type assetKind string

const (
	assetFile assetKind = "file"
	assetZip  assetKind = "zip"
)

type assetSpec struct {
	Label       string
	ObjectKey   string
	Path        string
	Kind        assetKind
	Mode        os.FileMode
	ContentType string
}

type SyncReport struct {
	Restored []string
	Mirrored []string
}

func SyncDefaultState(settings backup.Settings) (SyncReport, error) {
	report := SyncReport{}
	restored, err := RestoreDefaultState(settings)
	if err != nil {
		return report, err
	}
	report.Restored = restored
	mirrored, err := MirrorDefaultState(settings)
	if err != nil {
		return report, err
	}
	report.Mirrored = mirrored
	return report, nil
}

func MirrorDefaultState(settings backup.Settings) ([]string, error) {
	return mirrorAssets(settings, defaultAssets()...)
}

func RestoreDefaultState(settings backup.Settings) ([]string, error) {
	return restoreMissingAssets(settings, defaultAssets()...)
}

func MirrorCompany(settings backup.Settings, path string) error {
	return mirrorFileAsset(settings, assetSpec{
		Label:       "company.json",
		ObjectKey:   companyObjectKey,
		Path:        path,
		Kind:        assetFile,
		Mode:        0o600,
		ContentType: defaultJSONContentType,
	})
}

func RestoreCompanyIfMissing(settings backup.Settings, path string) (bool, error) {
	return restoreMissingFileAsset(settings, assetSpec{
		Label:       "company.json",
		ObjectKey:   companyObjectKey,
		Path:        path,
		Kind:        assetFile,
		Mode:        0o600,
		ContentType: defaultJSONContentType,
	})
}

func MirrorOnboarded(settings backup.Settings, path string) error {
	return mirrorFileAsset(settings, assetSpec{
		Label:       "onboarded.json",
		ObjectKey:   onboardedObjectKey,
		Path:        path,
		Kind:        assetFile,
		Mode:        0o600,
		ContentType: defaultJSONContentType,
	})
}

func RestoreOnboardedIfMissing(settings backup.Settings, path string) (bool, error) {
	return restoreMissingFileAsset(settings, assetSpec{
		Label:       "onboarded.json",
		ObjectKey:   onboardedObjectKey,
		Path:        path,
		Kind:        assetFile,
		Mode:        0o600,
		ContentType: defaultJSONContentType,
	})
}

func defaultAssets() []assetSpec {
	return []assetSpec{
		{
			Label:       "company.json",
			ObjectKey:   companyObjectKey,
			Path:        filepath.Join(config.RuntimeHomeDir(), ".wuphf", "company.json"),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultJSONContentType,
		},
		{
			Label:       "onboarded.json",
			ObjectKey:   onboardedObjectKey,
			Path:        filepath.Join(config.RuntimeHomeDir(), ".wuphf", "onboarded.json"),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultJSONContentType,
		},
		{
			Label:       "cloud-backup-bootstrap.json",
			ObjectKey:   bootstrapObjectKey,
			Path:        filepath.Join(filepath.Dir(config.ConfigPath()), "cloud-backup-bootstrap.json"),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultJSONContentType,
		},
		{
			Label:       ".codex/auth.json",
			ObjectKey:   codexAuthObjectKey,
			Path:        filepath.Join(globalHomeDir(), ".codex", "auth.json"),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultJSONContentType,
		},
		{
			Label:       ".codex/config.toml",
			ObjectKey:   codexConfigObjectKey,
			Path:        filepath.Join(globalHomeDir(), ".codex", "config.toml"),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultTOMLContentType,
		},
		{
			Label:       ".codex/skills",
			ObjectKey:   codexSkillsObjectKey,
			Path:        filepath.Join(globalHomeDir(), ".codex", "skills"),
			Kind:        assetZip,
			Mode:        0o700,
			ContentType: defaultZipContentType,
		},
		{
			Label:       ".agents/skills",
			ObjectKey:   agentsSkillsObjectKey,
			Path:        filepath.Join(globalHomeDir(), ".agents", "skills"),
			Kind:        assetZip,
			Mode:        0o700,
			ContentType: defaultZipContentType,
		},
		{
			Label:       "google-adc",
			ObjectKey:   gcloudADCObjectKey,
			Path:        googleApplicationDefaultCredentialsPath(),
			Kind:        assetFile,
			Mode:        0o600,
			ContentType: defaultJSONContentType,
		},
	}
}

func mirrorAssets(settings backup.Settings, assets ...assetSpec) ([]string, error) {
	if !settings.Enabled() {
		return nil, nil
	}
	var mirrored []string
	for _, asset := range assets {
		var (
			ok  bool
			err error
		)
		switch asset.Kind {
		case assetFile:
			ok, err = mirrorFileAssetPresence(settings, asset)
		case assetZip:
			ok, err = mirrorZipAssetPresence(settings, asset)
		default:
			continue
		}
		if err != nil {
			return mirrored, err
		}
		if ok {
			mirrored = append(mirrored, asset.Label)
		}
	}
	return mirrored, nil
}

func restoreMissingAssets(settings backup.Settings, assets ...assetSpec) ([]string, error) {
	if !settings.Enabled() {
		return nil, nil
	}
	var restored []string
	for _, asset := range assets {
		var (
			ok  bool
			err error
		)
		switch asset.Kind {
		case assetFile:
			ok, err = restoreMissingFileAsset(settings, asset)
		case assetZip:
			ok, err = restoreMissingZipAsset(settings, asset)
		default:
			continue
		}
		if err != nil {
			return restored, err
		}
		if ok {
			restored = append(restored, asset.Label)
		}
	}
	return restored, nil
}

func mirrorFileAsset(settings backup.Settings, asset assetSpec) error {
	_, err := mirrorFileAssetPresence(settings, asset)
	return err
}

func mirrorFileAssetPresence(settings backup.Settings, asset assetSpec) (bool, error) {
	if !settings.Enabled() || strings.TrimSpace(asset.Path) == "" {
		return false, nil
	}
	data, err := os.ReadFile(asset.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := backup.DeleteObject(nil, settings, asset.ObjectKey); err != nil && !backup.IsNotFound(err) {
				return false, err
			}
			return false, nil
		}
		return false, err
	}
	if err := backup.MirrorBytes(nil, settings, asset.ObjectKey, data, asset.ContentTypeOrDefault()); err != nil {
		return false, err
	}
	return true, nil
}

func restoreMissingFileAsset(settings backup.Settings, asset assetSpec) (bool, error) {
	if !settings.Enabled() || strings.TrimSpace(asset.Path) == "" {
		return false, nil
	}
	if _, err := os.Stat(asset.Path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	data, err := backup.ReadBytes(nil, settings, asset.ObjectKey)
	if err != nil {
		if backup.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(asset.Path), 0o700); err != nil {
		return false, err
	}
	if err := writeFileAtomically(asset.Path, data, asset.ModeOrDefault()); err != nil {
		return false, err
	}
	return true, nil
}

func mirrorZipAsset(settings backup.Settings, asset assetSpec) error {
	_, err := mirrorZipAssetPresence(settings, asset)
	return err
}

func mirrorZipAssetPresence(settings backup.Settings, asset assetSpec) (bool, error) {
	if !settings.Enabled() || strings.TrimSpace(asset.Path) == "" {
		return false, nil
	}
	hasEntries, err := directoryHasEntries(asset.Path)
	if err != nil {
		return false, err
	}
	if !hasEntries {
		if err := backup.DeleteObject(nil, settings, asset.ObjectKey); err != nil && !backup.IsNotFound(err) {
			return false, err
		}
		return false, nil
	}
	payload, err := zipDirectory(asset.Path)
	if err != nil {
		return false, err
	}
	if err := backup.MirrorBytes(nil, settings, asset.ObjectKey, payload, asset.ContentTypeOrDefault()); err != nil {
		return false, err
	}
	return true, nil
}

func restoreMissingZipAsset(settings backup.Settings, asset assetSpec) (bool, error) {
	if !settings.Enabled() || strings.TrimSpace(asset.Path) == "" {
		return false, nil
	}
	hasEntries, err := directoryHasEntries(asset.Path)
	if err != nil {
		return false, err
	}
	if hasEntries {
		return false, nil
	}
	payload, err := backup.ReadBytes(nil, settings, asset.ObjectKey)
	if err != nil {
		if backup.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.MkdirAll(asset.Path, asset.ModeOrDefault()); err != nil {
		return false, err
	}
	if err := unzipInto(asset.Path, payload); err != nil {
		return false, err
	}
	return true, nil
}

func zipDirectory(root string) ([]byte, error) {
	root = filepath.Clean(root)
	paths := make([]string, 0, 16)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Stat(path)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		header.Name = rel
		header.Method = zip.Deflate
		fileWriter, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		file, err := os.Open(path)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := io.Copy(fileWriter, file); err != nil {
			_ = file.Close()
			_ = writer.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unzipInto(root string, payload []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rel := filepath.Clean(filepath.FromSlash(file.Name))
		if rel == "." || strings.HasPrefix(rel, "..") {
			return errors.New("invalid zip entry path")
		}
		dest := filepath.Join(root, rel)
		if !strings.HasPrefix(dest, root+string(os.PathSeparator)) && dest != root {
			return errors.New("zip entry escaped destination")
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if perm := file.Mode().Perm(); perm != 0 {
			mode = perm
		}
		if err := writeFileAtomically(dest, data, mode); err != nil {
			return err
		}
	}
	return nil
}

func directoryHasEntries(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, errors.New("path is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

func globalHomeDir() string {
	if raw := strings.TrimSpace(os.Getenv("WUPHF_GLOBAL_HOME")); raw != "" {
		if abs, err := filepath.Abs(raw); err == nil && strings.TrimSpace(abs) != "" {
			return abs
		}
		return raw
	}
	return strings.TrimSpace(config.RuntimeHomeDir())
}

func googleApplicationDefaultCredentialsPath() string {
	if raw := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); raw != "" {
		return raw
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "gcloud", "application_default_credentials.json")
		}
		if home := globalHomeDir(); home != "" {
			return filepath.Join(home, "AppData", "Roaming", "gcloud", "application_default_credentials.json")
		}
		return ""
	}
	if home := globalHomeDir(); home != "" {
		return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	}
	return ""
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (a assetSpec) ContentTypeOrDefault() string {
	if strings.TrimSpace(a.ContentType) != "" {
		return a.ContentType
	}
	return defaultBinaryContentType
}

func (a assetSpec) ModeOrDefault() os.FileMode {
	if a.Mode != 0 {
		return a.Mode
	}
	return 0o600
}
