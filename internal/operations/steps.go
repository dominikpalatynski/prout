package operations

import "fmt"

const (
	StepSourceMaterialization = "source_materialization"
	StepWorkspaceCleanup      = "workspace_cleanup"

	StepStatePending    = "pending"
	StepStateInProgress = "in_progress"
	StepStateCompleted  = "completed"
	StepStateFailed     = "failed"
)

type StepStatus struct {
	Name  string
	State string
}

type stepTransition struct {
	from StepStatus
	to   StepStatus
}

type stepMachine struct {
	initial     StepStatus
	transitions map[stepTransition]struct{}
}

var stepMachines = map[string]stepMachine{
	TypePreviewStart: newStepMachine(
		StepStatus{Name: StepSourceMaterialization, State: StepStatePending},
		stepTransition{
			from: StepStatus{Name: StepSourceMaterialization, State: StepStatePending},
			to:   StepStatus{Name: StepSourceMaterialization, State: StepStateInProgress},
		},
		stepTransition{
			from: StepStatus{Name: StepSourceMaterialization, State: StepStatePending},
			to:   StepStatus{Name: StepSourceMaterialization, State: StepStateCompleted},
		},
		stepTransition{
			from: StepStatus{Name: StepSourceMaterialization, State: StepStatePending},
			to:   StepStatus{Name: StepSourceMaterialization, State: StepStateFailed},
		},
		stepTransition{
			from: StepStatus{Name: StepSourceMaterialization, State: StepStateInProgress},
			to:   StepStatus{Name: StepSourceMaterialization, State: StepStateCompleted},
		},
		stepTransition{
			from: StepStatus{Name: StepSourceMaterialization, State: StepStateInProgress},
			to:   StepStatus{Name: StepSourceMaterialization, State: StepStateFailed},
		},
	),
	TypePreviewCleanupSuperseded: newStepMachine(
		StepStatus{Name: StepWorkspaceCleanup, State: StepStatePending},
		stepTransition{
			from: StepStatus{Name: StepWorkspaceCleanup, State: StepStatePending},
			to:   StepStatus{Name: StepWorkspaceCleanup, State: StepStateInProgress},
		},
		stepTransition{
			from: StepStatus{Name: StepWorkspaceCleanup, State: StepStatePending},
			to:   StepStatus{Name: StepWorkspaceCleanup, State: StepStateCompleted},
		},
		stepTransition{
			from: StepStatus{Name: StepWorkspaceCleanup, State: StepStatePending},
			to:   StepStatus{Name: StepWorkspaceCleanup, State: StepStateFailed},
		},
		stepTransition{
			from: StepStatus{Name: StepWorkspaceCleanup, State: StepStateInProgress},
			to:   StepStatus{Name: StepWorkspaceCleanup, State: StepStateCompleted},
		},
		stepTransition{
			from: StepStatus{Name: StepWorkspaceCleanup, State: StepStateInProgress},
			to:   StepStatus{Name: StepWorkspaceCleanup, State: StepStateFailed},
		},
	),
}

func newStepMachine(initial StepStatus, transitions ...stepTransition) stepMachine {
	allowed := make(map[stepTransition]struct{}, len(transitions))
	for _, transition := range transitions {
		allowed[transition] = struct{}{}
	}
	return stepMachine{
		initial:     initial,
		transitions: allowed,
	}
}

func InitialStepForOperation(operationType string) (StepStatus, error) {
	machine, ok := stepMachines[operationType]
	if !ok {
		return StepStatus{}, fmt.Errorf("step machine is undefined for operation type %q", operationType)
	}
	return machine.initial, nil
}

func ValidateStepTransition(operationType string, from, to StepStatus) error {
	machine, ok := stepMachines[operationType]
	if !ok {
		return fmt.Errorf("step machine is undefined for operation type %q", operationType)
	}

	if _, allowed := machine.transitions[stepTransition{from: from, to: to}]; !allowed {
		return fmt.Errorf(
			"invalid step transition for %s: %s/%s -> %s/%s",
			operationType,
			from.Name,
			from.State,
			to.Name,
			to.State,
		)
	}
	return nil
}
