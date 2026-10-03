package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

type UpdateAgent interface {
	Call(context.Context, string, string, any, any) error
}

type UnixUpdateAgent struct {
	socket string
	token  string
	client *http.Client
}

func NewUnixUpdateAgent(socket, tokenFile string) (*UnixUpdateAgent, error) {
	if !filepath.IsAbs(socket) || !filepath.IsAbs(tokenFile) {
		return nil, errors.New("UPDATER_NOT_CONFIGURED")
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil || len(data) > 8192 || strings.TrimSpace(string(data)) == "" {
		return nil, errors.New("UPDATER_CREDENTIAL_UNAVAILABLE")
	}
	token := strings.TrimSpace(string(data))
	if strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("UPDATER_CREDENTIAL_INVALID")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &UnixUpdateAgent{socket: socket, token: token, client: &http.Client{Transport: transport, Timeout: 2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (agent *UnixUpdateAgent) Call(ctx context.Context, method, path string, input any, output any) error {
	if !strings.HasPrefix(path, "/v1/") || strings.Contains(path, "..") || strings.ContainsAny(path, "?#\\") {
		return errors.New("INVALID_UPDATER_ROUTE")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://updater"+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+agent.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := agent.client.Do(request)
	if err != nil {
		return errors.New("UPDATER_UNAVAILABLE")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		return errors.New("UPDATER_RESPONSE_INVALID")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &failure) == nil && len(failure.Error) <= 96 &&
			strings.Trim(failure.Error, "ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789") == "" && failure.Error != "" {
			return errors.New(failure.Error)
		}
		return errors.New("UPDATER_REQUEST_REJECTED")
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return errors.New("UPDATER_RESPONSE_INVALID")
	}
	return nil
}

type CustomRelease struct {
	ManifestHash string           `json:"manifest_hash"`
	Manifest     release.Manifest `json:"manifest"`
	ReleaseID    int64            `json:"release_id"`
}

type AgentCapabilities struct {
	Protocol          int  `json:"protocol"`
	ActivationEnabled bool `json:"activation_enabled"`
	Bootstrapped      bool `json:"bootstrapped"`
	Installed         struct {
		ManifestHash string           `json:"manifest_hash"`
		Manifest     release.Manifest `json:"manifest"`
	} `json:"installed"`
}

func (service *UpdateService) WithAgent(agent UpdateAgent) *UpdateService {
	service.agent = agent
	return service
}

func (service *UpdateService) Agent() UpdateAgent { return service.agent }

func (service *UpdateService) IsCustomDistribution() bool {
	return strings.Contains(service.currentVersion, "-r")
}

func (service *UpdateService) LocalVersionInfo() *UpdateInfo {
	info := &UpdateInfo{CurrentVersion: service.currentVersion, LatestVersion: service.currentVersion,
		BuildType: service.buildType, Commit: service.commit, CheckStatus: "unknown"}
	if service.IsCustomDistribution() {
		info.Distribution, info.UpdateSource, info.InstallationMode = "sshzy", release.Repository, "custom-ghcr"
		if version, err := release.ParseVersion(service.currentVersion); err == nil {
			info.UpstreamVersion = strings.TrimPrefix(version.OfficialTag(), "v")
		}
	}
	return info
}

func (service *UpdateService) CustomCapabilities(ctx context.Context) (AgentCapabilities, error) {
	var capabilities AgentCapabilities
	if !service.IsCustomDistribution() || service.buildType != "release" || service.agent == nil {
		return capabilities, errors.New("UPDATER_NOT_CONFIGURED")
	}
	if err := service.agent.Call(ctx, http.MethodGet, "/v1/capabilities", nil, &capabilities); err != nil {
		return capabilities, err
	}
	if capabilities.Protocol != release.Protocol || !capabilities.Bootstrapped ||
		capabilities.Installed.Manifest.Version != service.currentVersion ||
		(service.commit != "" && capabilities.Installed.Manifest.SourceSHA != service.commit) {
		return capabilities, errors.New("UPDATER_BOOTSTRAP_REQUIRED")
	}
	return capabilities, nil
}

func (service *UpdateService) CustomReleases(ctx context.Context) ([]CustomRelease, error) {
	if _, err := service.CustomCapabilities(ctx); err != nil {
		return nil, err
	}
	var result struct {
		Releases []CustomRelease `json:"releases"`
	}
	if err := service.agent.Call(ctx, http.MethodGet, "/v1/releases", nil, &result); err != nil {
		return nil, err
	}
	for _, candidate := range result.Releases {
		if candidate.Manifest.Repository != release.Repository {
			return nil, errors.New("UPDATER_RELEASE_SOURCE_INVALID")
		}
		if _, err := release.ParseVersion(candidate.Manifest.Version); err != nil {
			return nil, errors.New("UPDATER_RELEASE_VERSION_INVALID")
		}
	}
	return result.Releases, nil
}

func (service *UpdateService) checkCustomUpdate(ctx context.Context) (*UpdateInfo, error) {
	info := service.LocalVersionInfo()
	capabilities, err := service.CustomCapabilities(ctx)
	if err != nil {
		info.Warning = "Custom updater is unavailable or requires initial installation"
		return info, nil
	}
	targets, err := service.CustomReleases(ctx)
	if err != nil {
		info.Warning = "Unable to verify the custom release source"
		return info, nil
	}
	current, _ := release.ParseVersion(service.currentVersion)
	best := current
	for _, candidate := range targets {
		version, _ := release.ParseVersion(candidate.Manifest.Version)
		if version.Compare(best) > 0 {
			best = version
			info.LatestVersion, info.CustomRelease = candidate.Manifest.Version, &candidate
		}
	}
	info.HasUpdate, info.CanUpdate, info.CheckStatus = best.Compare(current) > 0, capabilities.ActivationEnabled, "verified"
	if !capabilities.ActivationEnabled {
		info.Warning = "Activation is disabled by the deployment administrator"
	}
	if service.githubClient != nil {
		official, err := service.githubClient.FetchLatestRelease(ctx, githubRepo)
		if err == nil && official != nil {
			info.OfficialNotice = &OfficialUpdateNotice{Version: strings.TrimPrefix(official.TagName, "v"), URL: official.HTMLURL,
				HasUpdate: compareVersions(info.UpstreamVersion, official.TagName) < 0}
		}
	}
	return info, nil
}

func ValidControlAuthorization(header string) bool {
	file := os.Getenv("SSHZY_UPDATE_AGENT_TOKEN_FILE")
	if file == "" {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil || len(data) > 8192 {
		return false
	}
	token := strings.TrimSpace(string(data))
	return token != "" && subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+token)) == 1
}
