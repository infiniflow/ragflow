package runnable

import (
	"context"
	"sync"
	"testing"
)

func TestNewRunnableFunc(t *testing.T) {
	fn := func(ctx context.Context, input string) (string, error) {
		return "hello " + input, nil
	}
	r := NewRunnableFunc(fn)
	result, err := r.Invoke(context.Background(), "world")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %s", result)
	}
}

func TestRunnableFunc_WithOptions(t *testing.T) {
	fn := func(ctx context.Context, input string) (string, error) { return input, nil }
	r := NewRunnableFunc(fn, WithName[string, string]("myfunc"), WithDescription[string, string]("desc"))
	s := r.GetSchema()
	if s.Name != "myfunc" || s.Description != "desc" {
		t.Errorf("unexpected schema: %+v", s)
	}
}

func TestRunnableFunc_Batch(t *testing.T) {
	fn := func(ctx context.Context, input int) (int, error) { return input * 2, nil }
	r := NewRunnableFunc(fn)
	outputs, errs := r.Batch(context.Background(), []int{1, 2, 3})
	if len(outputs) != 3 || outputs[0] != 2 || outputs[2] != 6 {
		t.Errorf("expected [2,4,6], got %v", outputs)
	}
	_ = errs
}

func TestRunnableFunc_Stream(t *testing.T) {
	fn := func(ctx context.Context, input string) (string, error) { return input, nil }
	r := NewRunnableFunc(fn)
	ch := r.Stream(context.Background(), "test")
	val, ok := <-ch
	if !ok || val != "test" {
		t.Errorf("expected 'test', got %v (ok=%v)", val, ok)
	}
}

func TestRunnableFunc_GetSchema(t *testing.T) {
	fn := func(ctx context.Context, input string) (string, error) { return input, nil }
	r := NewRunnableFunc(fn, WithName[string, string]("schema_test"))
	if s := r.GetSchema(); s.Name != "schema_test" {
		t.Errorf("expected 'schema_test', got %s", s.Name)
	}
}

func TestRunnableFunc_Error(t *testing.T) {
	r := NewRunnableFunc(func(ctx context.Context, input string) (string, error) {
		return "", &RunnableError{Message: "failed"}
	})
	_, err := r.Invoke(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnableSeq(t *testing.T) {
	step1 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		s, _ := input.(string)
		return s + "_a", nil
	})
	step2 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		s, _ := input.(string)
		return s + "_b", nil
	})
	seq, err := NewRunnableSeq(step1, step2)
	if err != nil {
		t.Fatalf("NewRunnableSeq: %v", err)
	}
	result, err := seq.Invoke(context.Background(), "start")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	s, _ := result.(string)
	if s != "start_a_b" {
		t.Errorf("expected 'start_a_b', got %s", s)
	}
}

func TestRunnableSeq_Negative(t *testing.T) {
	_, err := NewRunnableSeq()
	if err == nil {
		t.Error("expected error for empty sequence")
	}
}

func TestRunnableParallel(t *testing.T) {
	r1 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		n, _ := input.(int)
		return n * 10, nil
	})
	r2 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		n, _ := input.(int)
		return n * 20, nil
	})
	par := NewRunnableParallel(map[string]Runnable[any, any]{"r1": r1, "r2": r2})
	results, err := par.Invoke(context.Background(), 5)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m, _ := results.(map[string]any)
	if len(m) != 2 {
		t.Errorf("expected 2 results, got %d", len(m))
	}
}

func TestRunnableMap(t *testing.T) {
	inner := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		return "proc:" + anyToString(input), nil
	})
	mapped := NewRunnableMap(inner,
		func(ctx context.Context, input any) (any, error) { return input, nil },
		func(ctx context.Context, output any) (any, error) { return output, nil },
	)
	result, _ := mapped.Invoke(context.Background(), "data")
	s, _ := result.(string)
	if s != "proc:data" {
		t.Errorf("expected 'proc:data', got %s", s)
	}
}

func TestRunnableMap_Batch(t *testing.T) {
	inner := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		n, _ := input.(int)
		return n + 1, nil
	})
	mapped := NewRunnableMap(inner,
		func(ctx context.Context, input any) (any, error) { return input, nil },
		func(ctx context.Context, output any) (any, error) { return output, nil },
	)
	outputs, errs := mapped.Batch(context.Background(), []any{1, 2, 3})
	if len(outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(outputs))
	}
	_ = errs
}

func TestRunnableBuilder(t *testing.T) {
	base := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		return "built:" + anyToString(input), nil
	})
	r := NewRunnableBuilder(base).Build()
	result, _ := r.Invoke(context.Background(), "value")
	s, _ := result.(string)
	if s != "built:value" {
		t.Errorf("expected 'built:value', got %s", s)
	}
}

func TestRunnableBuilder_Then(t *testing.T) {
	step1 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		return anyToString(input) + "_a", nil
	})
	step2 := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		return anyToString(input) + "_b", nil
	})
	b, err := NewRunnableBuilder(step1).Then(step2)
	if err != nil {
		t.Fatalf("Then: %v", err)
	}
	result, _ := b.Build().Invoke(context.Background(), "start")
	s, _ := result.(string)
	if s != "start_a_b" {
		t.Errorf("expected 'start_a_b', got %s", s)
	}
}

func TestRunnableBuilder_Map(t *testing.T) {
	inner := NewRunnableFunc(func(ctx context.Context, input any) (any, error) {
		return anyToString(input) + "_inner", nil
	})
	b := NewRunnableBuilder(inner).Map(
		func(ctx context.Context, input any) (any, error) { return input, nil },
		func(ctx context.Context, output any) (any, error) { return output, nil },
	)
	result, _ := b.Build().Invoke(context.Background(), "data")
	s, _ := result.(string)
	if s != "data_inner" {
		t.Errorf("expected 'data_inner', got %s", s)
	}
}

func TestRunnableFunc_Concurrent(t *testing.T) {
	fn := func(ctx context.Context, input int) (int, error) { return input * 2, nil }
	r := NewRunnableFunc(fn)
	var wg sync.WaitGroup
	ch := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			res, _ := r.Invoke(context.Background(), val)
			ch <- res
		}(i)
	}
	wg.Wait()
	close(ch)
	count := 0
	for range ch {
		count++
	}
	if count != 20 {
		t.Errorf("expected 20 results, got %d", count)
	}
}

func anyToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
