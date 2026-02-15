package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ── Keychain Data Types ──

type OidcToken struct {
	AccessToken  string   `json:"access_token"`
	ExpiresAt    string   `json:"expires_at"`
	RefreshToken string   `json:"refresh_token"`
	Region       string   `json:"region"`
	StartURL     *string  `json:"start_url,omitempty"`
	OAuthFlow    *string  `json:"oauth_flow,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Provider     *string  `json:"provider,omitempty"`
	ProfileArn   *string  `json:"profile_arn,omitempty"`
}

type DeviceRegistration struct {
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret"`
	ClientSecretExpiresAt *string  `json:"client_secret_expires_at,omitempty"`
	Region                string   `json:"region"`
	OAuthFlow             *string  `json:"oauth_flow,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
}

type KeychainCredentials struct {
	Token  OidcToken           `json:"token"`
	Device *DeviceRegistration `json:"device,omitempty"`
	Source string              `json:"source"`
}

// ── macOS Keychain ──

func macReadGenericPassword(service string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("keychain read failed for %s: %v", service, err)
	}
	return string(out), nil
}

func macTryReadToken(service string) *OidcToken {
	data, err := macReadGenericPassword(service)
	if err != nil {
		return nil
	}
	var token OidcToken
	if json.Unmarshal([]byte(data), &token) != nil {
		return nil
	}
	if token.Region == "" {
		token.Region = "us-east-1"
	}
	return &token
}

func macTryReadDevice(service string) *DeviceRegistration {
	data, err := macReadGenericPassword(service)
	if err != nil {
		return nil
	}
	var dev DeviceRegistration
	if json.Unmarshal([]byte(data), &dev) != nil {
		return nil
	}
	return &dev
}

func macReadAllCredentials() []KeychainCredentials {
	var results []KeychainCredentials

	// 1. Primary: read from ~/.aws/sso/cache/ (Kiro IDE storage)
	results = append(results, readAwsSsoCacheAll()...)

	// 2. Fallback: macOS Keychain (Kiro CLI)
	existingRTs := map[string]bool{}
	for _, c := range results {
		existingRTs[c.Token.RefreshToken] = true
	}

	if token := macTryReadToken("kirocli:odic:token"); token != nil {
		if !existingRTs[token.RefreshToken] {
			device := macTryReadDevice("kirocli:odic:device-registration")
			results = append(results, KeychainCredentials{Token: *token, Device: device, Source: "idc (keychain)"})
		}
	}
	if token := macTryReadToken("kirocli:social:token"); token != nil {
		if !existingRTs[token.RefreshToken] {
			device := macTryReadDevice("kirocli:social:device-registration")
			results = append(results, KeychainCredentials{Token: *token, Device: device, Source: "social (keychain)"})
		}
	}

	return results
}

// ── Cross-platform: Read from ~/.aws/sso/cache/ (Kiro IDE storage) ──
// Kiro IDE stores tokens in ~/.aws/sso/cache/kiro-auth-token.json
// Additional accounts: kiro-auth-token.{email}.json
// Device registration: {clientIdHash}.json with clientId/clientSecret

func readAwsSsoCacheAll() []KeychainCredentials {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	cacheDir := filepath.Join(home, ".aws", "sso", "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil
	}

	var results []KeychainCredentials
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "kiro-auth-token") || !strings.HasSuffix(name, ".json") {
			continue
		}

		tokenData, err := os.ReadFile(filepath.Join(cacheDir, name))
		if err != nil {
			continue
		}

		var tokenJSON map[string]interface{}
		if json.Unmarshal(tokenData, &tokenJSON) != nil {
			continue
		}

		refreshToken, _ := tokenJSON["refreshToken"].(string)
		if refreshToken == "" {
			continue
		}

		accessToken, _ := tokenJSON["accessToken"].(string)
		expiresAt, _ := tokenJSON["expiresAt"].(string)
		authMethod, _ := tokenJSON["authMethod"].(string)
		region, _ := tokenJSON["region"].(string)
		if region == "" {
			region = "us-east-1"
		}

		provider, _ := tokenJSON["provider"].(string)
		var providerPtr *string
		if provider != "" {
			providerPtr = &provider
		}

		token := OidcToken{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
			Region:       region,
			Provider:     providerPtr,
		}

		source := "social"
		var device *DeviceRegistration

		if strings.EqualFold(authMethod, "IdC") {
			source = "idc"
			clientIdHash, _ := tokenJSON["clientIdHash"].(string)
			if clientIdHash != "" {
				regPath := filepath.Join(cacheDir, clientIdHash+".json")
				if regData, err := os.ReadFile(regPath); err == nil {
					var reg map[string]interface{}
					if json.Unmarshal(regData, &reg) == nil {
						cid, _ := reg["clientId"].(string)
						csec, _ := reg["clientSecret"].(string)
						if cid != "" && csec != "" {
							device = &DeviceRegistration{
								ClientID:     cid,
								ClientSecret: csec,
								Region:       region,
							}
						}
					}
				}
			}
		}

		// Source label with provider info
		if provider != "" {
			source = fmt.Sprintf("%s (%s)", source, provider)
		}

		results = append(results, KeychainCredentials{Token: token, Device: device, Source: source})
	}

	return results
}

func winTryReadFromFile(name string) *string {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "kiro", name),
		filepath.Join(home, "AppData", "Roaming", "Kiro", name),
		filepath.Join(home, ".kiro", name),
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			s := string(data)
			return &s
		}
	}
	return nil
}

func winReadAllCredentials() []KeychainCredentials {
	var results []KeychainCredentials

	// 1. Primary: read from ~/.aws/sso/cache/ (Kiro IDE storage)
	results = append(results, readAwsSsoCacheAll()...)

	// 2. Fallback: try file-based credentials in app data dirs
	if len(results) == 0 {
		if tokenStr := winTryReadFromFile("credentials.json"); tokenStr != nil {
			var token OidcToken
			if json.Unmarshal([]byte(*tokenStr), &token) == nil {
				if token.Region == "" {
					token.Region = "us-east-1"
				}
				results = append(results, KeychainCredentials{Token: token, Source: "file"})
			}
		}
	}

	return results
}

// ── Public API ──

func readAllCredentials() []KeychainCredentials {
	switch runtime.GOOS {
	case "darwin":
		return macReadAllCredentials()
	case "windows":
		return winReadAllCredentials()
	default:
		return nil
	}
}

func readKiroCredentials() (*KeychainCredentials, error) {
	all := readAllCredentials()
	if len(all) == 0 {
		return nil, fmt.Errorf("无法从系统凭据存储读取 token (已检查 ~/.aws/sso/cache/ 和系统 Keychain)")
	}

	// Priority: IDE sources (no "keychain" in source) > Keychain sources
	// Within each group: idc > social, newer expiresAt preferred
	var ideSources, keychainSources []KeychainCredentials
	for _, c := range all {
		if strings.Contains(c.Source, "keychain") {
			keychainSources = append(keychainSources, c)
		} else {
			ideSources = append(ideSources, c)
		}
	}

	// Pick best from a group: prefer idc, then latest expiresAt
	pickBest := func(group []KeychainCredentials) *KeychainCredentials {
		var best *KeychainCredentials
		for i := range group {
			c := &group[i]
			if best == nil {
				best = c
				continue
			}
			bestIsIDC := strings.HasPrefix(best.Source, "idc")
			cIsIDC := strings.HasPrefix(c.Source, "idc")
			if cIsIDC && !bestIsIDC {
				best = c
			} else if cIsIDC == bestIsIDC && c.Token.ExpiresAt > best.Token.ExpiresAt {
				best = c
			}
		}
		return best
	}

	if b := pickBest(ideSources); b != nil {
		return b, nil
	}
	if b := pickBest(keychainSources); b != nil {
		return b, nil
	}
	return &all[0], nil
}

func writeKeychainCredentials(creds *KeychainCredentials) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "credentials.json")

	hasDevice := creds.Device != nil && creds.Device.ClientID != "" && creds.Device.ClientSecret != ""
	authMethod := "social"
	if hasDevice {
		authMethod = "idc"
	}

	var clientID, clientSecret *string
	if creds.Device != nil {
		clientID = &creds.Device.ClientID
		clientSecret = &creds.Device.ClientSecret
	}

	cf := CredentialsFile{
		AccessToken:  &creds.Token.AccessToken,
		RefreshToken: creds.Token.RefreshToken,
		ExpiresAt:    creds.Token.ExpiresAt,
		AuthMethod:   authMethod,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Region:       &creds.Token.Region,
	}

	data, _ := json.MarshalIndent(cf, "", "  ")
	return os.WriteFile(path, data, 0644)
}
