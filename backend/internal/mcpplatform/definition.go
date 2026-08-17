package mcpplatform

import (
	"encoding/json"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

func revisionFromDefinition(installationID uint64, revisionNumber int, userID uint64, definition InstallationDefinition) (models.MCPInstallationRevision, error) {
	commandJSON, err := json.Marshal(definition.Command)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	argsJSON, err := json.Marshal(definition.Args)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	configJSON, err := json.Marshal(definition.Config)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	allowlistJSON, err := json.Marshal(definition.NetworkAllowlist)
	if err != nil {
		return models.MCPInstallationRevision{}, err
	}
	return models.MCPInstallationRevision{
		InstallationID:       installationID,
		Revision:             revisionNumber,
		Transport:            strings.ToLower(strings.TrimSpace(definition.Transport)),
		ImageRef:             strings.TrimSpace(definition.ImageRef),
		EndpointURL:          strings.TrimSpace(definition.EndpointURL),
		CommandJSON:          string(commandJSON),
		ArgsJSON:             string(argsJSON),
		ConfigJSON:           string(configJSON),
		NetworkAllowlistJSON: string(allowlistJSON),
		ScanStatus:           "pending",
		ScanReportJSON:       "{}",
		CreatedBy:            userID,
	}, nil
}

func definitionFromRevision(revision models.MCPInstallationRevision) (InstallationDefinition, error) {
	definition := InstallationDefinition{
		Transport:   revision.Transport,
		ImageRef:    revision.ImageRef,
		EndpointURL: revision.EndpointURL,
	}
	if err := json.Unmarshal([]byte(revision.CommandJSON), &definition.Command); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.ArgsJSON), &definition.Args); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.ConfigJSON), &definition.Config); err != nil {
		return InstallationDefinition{}, err
	}
	if err := json.Unmarshal([]byte(revision.NetworkAllowlistJSON), &definition.NetworkAllowlist); err != nil {
		return InstallationDefinition{}, err
	}
	return definition, nil
}
