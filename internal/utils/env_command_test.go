package utils

import "testing"

func TestCommandWithEnvAddsEnvironment(t *testing.T) {
	err := CommandWithEnv(
		[]string{"sh", "-c", `test "$PIG_TEST_COMMAND_ENV" = "mirror"`},
		[]string{"PIG_TEST_COMMAND_ENV=mirror"},
	)
	if err != nil {
		t.Fatalf("CommandWithEnv returned error: %v", err)
	}
}
