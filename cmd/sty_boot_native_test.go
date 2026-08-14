package cmd

import (
	"context"
	"errors"
	"io"
	"testing"

	stycli "pig/cli/sty"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestStyBootDefaultsToNative(t *testing.T) {
	origNative := pigstyBootNativeExec
	origEUID := pigstyBootEUID
	origMirror, origRegion, origKeep := pigstyBootMirror, pigstyBootRegion, pigstyBootKeep
	t.Cleanup(func() {
		pigstyBootNativeExec = origNative
		pigstyBootEUID = origEUID
		pigstyBootMirror, pigstyBootRegion, pigstyBootKeep = origMirror, origRegion, origKeep
	})
	pigstyBootMirror, pigstyBootRegion, pigstyBootKeep = true, "", false
	pigstyBootEUID = func() int { return 0 }
	var got stycli.NativeBootstrapOptions
	pigstyBootNativeExec = func(_ context.Context, opts stycli.NativeBootstrapOptions) *output.Result {
		got = opts
		return output.OK("ready", &stycli.BootData{Kind: stycli.BootResultKind})
	}
	if err := pigstyBootCmd.RunE(pigstyBootCmd, nil); err != nil {
		t.Fatal(err)
	}
	if got.Region != "china" || got.Keep {
		t.Fatalf("native options = %+v", got)
	}
}

func TestStyBootKeepDisablesDefaultOverwrite(t *testing.T) {
	origNative, origEUID := pigstyBootNativeExec, pigstyBootEUID
	origKeep := pigstyBootKeep
	t.Cleanup(func() {
		pigstyBootNativeExec, pigstyBootEUID = origNative, origEUID
		pigstyBootKeep = origKeep
	})
	pigstyBootKeep = true
	pigstyBootEUID = func() int { return 0 }
	var got stycli.NativeBootstrapOptions
	pigstyBootNativeExec = func(_ context.Context, opts stycli.NativeBootstrapOptions) *output.Result {
		got = opts
		return output.OK("ready", &stycli.BootData{Kind: stycli.BootResultKind})
	}
	if err := pigstyBootCmd.RunE(pigstyBootCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !got.Keep {
		t.Fatalf("keep options = %+v", got)
	}
}

func TestStyBootRemovedFlagsAreAbsent(t *testing.T) {
	for _, name := range []string{"overwrite", "raw"} {
		if flag := pigstyBootCmd.Flags().Lookup(name); flag != nil {
			t.Fatalf("removed --%s flag is still registered", name)
		}
	}
}

func TestStyBootNonRootReexecsOnce(t *testing.T) {
	origNative, origReexec, origEUID := pigstyBootNativeExec, pigstyBootReexec, pigstyBootEUID
	origKeep := pigstyBootKeep
	t.Cleanup(func() {
		pigstyBootNativeExec, pigstyBootReexec, pigstyBootEUID = origNative, origReexec, origEUID
		pigstyBootKeep = origKeep
	})
	pigstyBootKeep = false
	pigstyBootEUID = func() int { return 501 }
	calls := 0
	pigstyBootReexec = func([]string, io.Writer) stycli.ReexecOutcome {
		calls++
		return stycli.ReexecOutcome{Started: true}
	}
	pigstyBootNativeExec = func(context.Context, stycli.NativeBootstrapOptions) *output.Result {
		t.Fatal("non-root parent ran native engine")
		return nil
	}
	if err := pigstyBootCmd.RunE(pigstyBootCmd, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("sudo reexec calls = %d", calls)
	}
}

func TestStyBootNonRootPreservesTextChildFailure(t *testing.T) {
	origNative, origReexec, origEUID := pigstyBootNativeExec, pigstyBootReexec, pigstyBootEUID
	t.Cleanup(func() {
		pigstyBootNativeExec, pigstyBootReexec, pigstyBootEUID = origNative, origReexec, origEUID
	})
	pigstyBootEUID = func() int { return 501 }
	pigstyBootReexec = func([]string, io.Writer) stycli.ReexecOutcome {
		return stycli.ReexecOutcome{Started: true, ExitCode: 5, Err: errors.New("child failed")}
	}
	pigstyBootNativeExec = func(context.Context, stycli.NativeBootstrapOptions) *output.Result {
		t.Fatal("non-root parent ran native engine")
		return nil
	}

	err := pigstyBootCmd.RunE(pigstyBootCmd, nil)
	exitErr, ok := err.(*utils.ExitCodeError)
	if !ok || exitErr.Code != 5 || !exitErr.Silent {
		t.Fatalf("child failure = %#v, want silent exit 5", err)
	}
}
