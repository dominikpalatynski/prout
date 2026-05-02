package operations

import "testing"

func TestInitialStepForOperation(t *testing.T) {
	t.Parallel()

	step, err := InitialStepForOperation(TypePreviewStart)
	if err != nil {
		t.Fatalf("InitialStepForOperation() error = %v", err)
	}
	if step.Name != StepSourceMaterialization || step.State != StepStatePending {
		t.Fatalf("initial step = %#v, want source_materialization/pending", step)
	}
}

func TestValidateStepTransitionAllowsConfiguredTransition(t *testing.T) {
	t.Parallel()

	err := ValidateStepTransition(
		TypePreviewStart,
		StepStatus{Name: StepSourceMaterialization, State: StepStatePending},
		StepStatus{Name: StepSourceMaterialization, State: StepStateInProgress},
	)
	if err != nil {
		t.Fatalf("ValidateStepTransition() error = %v", err)
	}
}

func TestValidateStepTransitionAllowsCrossStepTransition(t *testing.T) {
	t.Parallel()

	err := ValidateStepTransition(
		TypePreviewStart,
		StepStatus{Name: StepSourceMaterialization, State: StepStateCompleted},
		StepStatus{Name: StepComposePreparation, State: StepStatePending},
	)
	if err != nil {
		t.Fatalf("ValidateStepTransition() error = %v", err)
	}
}

func TestValidateStepTransitionRejectsInvalidTransition(t *testing.T) {
	t.Parallel()

	err := ValidateStepTransition(
		TypePreviewStart,
		StepStatus{Name: StepSourceMaterialization, State: StepStateCompleted},
		StepStatus{Name: StepSourceMaterialization, State: StepStateInProgress},
	)
	if err == nil {
		t.Fatalf("ValidateStepTransition() error = nil, want non-nil")
	}
}

func TestCleanupOperationStartsWithRuntimeTeardown(t *testing.T) {
	t.Parallel()

	step, err := InitialStepForOperation(TypePreviewCleanupSuperseded)
	if err != nil {
		t.Fatalf("InitialStepForOperation() error = %v", err)
	}
	if step.Name != StepRuntimeTeardown || step.State != StepStatePending {
		t.Fatalf("initial cleanup step = %#v, want runtime_teardown/pending", step)
	}
}
