package cmd

import (
	"bufio"
	"fmt"
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
