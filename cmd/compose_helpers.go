package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Tech-Arch1tect/berth-cli/pkg/client"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

func updateComposeWithConfirm(c *client.Client, serverID int32, stackName string, changes *berth.ComposeChanges, skipConfirm bool) error {
	req := berth.NewUpdateComposeRequest(*changes)
	req.SetPreview(true)

	result, _, err := c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposePatch(c.Ctx, serverID, stackName).
		UpdateComposeRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("failed to preview changes: %w", err)
	}

	if result.HasOriginalYaml() && result.HasModifiedYaml() {
		displayDiff(result.GetOriginalYaml(), result.GetModifiedYaml())
	}

	if !skipConfirm {
		if !confirmApply() {
			fmt.Println("Cancelled.")
			os.Exit(0)
		}
	}

	req.SetPreview(false)
	_, _, err = c.API.ComposeAPI.ApiV1ServersServeridStacksStacknameComposePatch(c.Ctx, serverID, stackName).
		UpdateComposeRequest(*req).Execute()
	if err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}

	return nil
}

func displayDiff(original, modified string) {
	origLines := strings.Split(strings.TrimSpace(original), "\n")
	modLines := strings.Split(strings.TrimSpace(modified), "\n")

	origSet := make(map[string]bool)
	modSet := make(map[string]bool)
	for _, line := range origLines {
		origSet[line] = true
	}
	for _, line := range modLines {
		modSet[line] = true
	}

	fmt.Println()

	hasChanges := false
	for i := 0; i < len(origLines) || i < len(modLines); i++ {
		origLine, modLine := "", ""
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(modLines) {
			modLine = modLines[i]
		}

		if origLine == modLine {
			continue
		}

		if origLine != "" && !modSet[origLine] {
			fmt.Printf("- %s\n", origLine)
			hasChanges = true
		}

		if modLine != "" && !origSet[modLine] {
			fmt.Printf("+ %s\n", modLine)
			hasChanges = true
		}
	}

	if !hasChanges {
		fmt.Println("No changes detected.")
	}
	fmt.Println()
}

func confirmApply() bool {
	fmt.Print("Apply these changes? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(response)
	return response == "y" || response == "Y"
}

func updateComposeRaw(c *client.Client, serverID int32, stackName string, changes map[string]any, skipConfirm bool) error {

	serverURL := c.API.GetConfig().Servers[0].URL

	result, err := doComposeRawRequest(c, serverURL, serverID, stackName, changes, true)
	if err != nil {
		return fmt.Errorf("failed to preview changes: %w", err)
	}

	if result.OriginalYAML != "" && result.ModifiedYAML != "" {
		displayDiff(result.OriginalYAML, result.ModifiedYAML)
	}

	if !skipConfirm {
		if !confirmApply() {
			fmt.Println("Cancelled.")
			os.Exit(0)
		}
	}

	_, err = doComposeRawRequest(c, serverURL, serverID, stackName, changes, false)
	if err != nil {
		return fmt.Errorf("failed to apply changes: %w", err)
	}

	return nil
}

type rawUpdateResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	OriginalYAML string `json:"original_yaml,omitempty"`
	ModifiedYAML string `json:"modified_yaml,omitempty"`
}

func doComposeRawRequest(c *client.Client, serverURL string, serverID int32, stackName string, changes map[string]any, preview bool) (*rawUpdateResult, error) {
	payload := map[string]any{
		"changes": changes,
		"preview": preview,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/servers/%d/stacks/%s/compose", serverURL, serverID, stackName)

	req, err := http.NewRequestWithContext(c.Ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token := c.Ctx.Value(berth.ContextAccessToken); token != nil {
		req.Header.Set("Authorization", "Bearer "+token.(string))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.API.GetConfig().HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		if json.Unmarshal(body, &errResp) == nil {
			if msg, ok := errResp["error"].(string); ok {
				return nil, fmt.Errorf("API error: %s", msg)
			}
		}
		return nil, fmt.Errorf("request failed: %s - %s", resp.Status, string(body))
	}

	var result rawUpdateResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
