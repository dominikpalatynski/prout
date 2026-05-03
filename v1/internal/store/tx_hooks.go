package store

import "context"

type afterCommitKey struct{}

type afterCommitState struct {
	hooks []func()
}

func WithAfterCommit(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if afterCommitStateFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, afterCommitKey{}, &afterCommitState{})
}

func AfterCommit(ctx context.Context, fn func()) {
	if fn == nil {
		return
	}

	state := afterCommitStateFromContext(ctx)
	if state == nil {
		fn()
		return
	}

	state.hooks = append(state.hooks, fn)
}

func runAfterCommitHooks(ctx context.Context) {
	state := afterCommitStateFromContext(ctx)
	if state == nil {
		return
	}

	hooks := append([]func(){}, state.hooks...)
	state.hooks = state.hooks[:0]

	for _, hook := range hooks {
		hook()
	}
}

func afterCommitStateFromContext(ctx context.Context) *afterCommitState {
	if ctx == nil {
		return nil
	}

	state, _ := ctx.Value(afterCommitKey{}).(*afterCommitState)
	return state
}
