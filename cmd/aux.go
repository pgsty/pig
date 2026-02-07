package cmd

import (
	"fmt"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

// handleAuxResult renders structured output for auxiliary commands (status/version).
func handleAuxResult(result *output.Result) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	if err := output.Print(result); err != nil {
		return err
	}
	if !result.Success {
		return &utils.ExitCodeError{Code: result.ExitCode(), Err: fmt.Errorf("%s", result.Message)}
	}
	return nil
}

// handlePlanOutput renders a plan using the current global output format.
func handlePlanOutput(plan *output.Plan) error {
	if plan == nil {
		return fmt.Errorf("nil plan")
	}
	data, err := plan.Render(config.OutputFormat)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runStyLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_STY, command, args, params, fn)
}

func runExtLegacy(command string, args []string, params map[string]interface{}, fn func() error) error {
	return runLegacyStructured(output.MODULE_EXT, command, args, params, fn)
}
