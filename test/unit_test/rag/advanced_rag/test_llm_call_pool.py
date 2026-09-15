import asyncio

import pytest

from rag.advanced_rag.knowlege_compile.structure import LLMCallPool


class _Clock:
    def __init__(self):
        self.now = 0.0

    def __call__(self):
        return self.now

    def advance(self, seconds: float):
        self.now += seconds


class _ChatModel:
    def __init__(self, name: str, response: str = "ok"):
        self.llm_name = name
        self.model_config = {"llm_factory": "test", "llm_name": name}
        self.response = response

    async def async_chat(self, system, history, gen_conf=None, **kwargs):
        return self.response


async def _return(value):
    return value


def test_llm_call_pool_halves_model_concurrency_once_per_cooldown():
    async def exercise():
        clock = _Clock()
        pool = LLMCallPool(20, decrease_cooldown=5, rate_limit_retries=0, clock=clock)
        model = _ChatModel("limited")

        async def limited_call():
            return "**ERROR**: RATE_LIMIT_EXCEEDED - 429 too many requests"

        await pool.call(limited_call, model_key=pool._model_key(model), priority=10, label="limited")
        assert pool.concurrency_for(model) == 10

        await pool.call(limited_call, model_key=pool._model_key(model), priority=10, label="limited")
        assert pool.concurrency_for(model) == 10

        clock.advance(5)
        await pool.call(limited_call, model_key=pool._model_key(model), priority=10, label="limited")
        assert pool.concurrency_for(model) == 5

    asyncio.run(exercise())


def test_pooled_chat_model_reports_rate_limit_results_to_the_pool():
    async def exercise():
        pool = LLMCallPool(8, decrease_cooldown=0, rate_limit_retries=0)
        model = _ChatModel("wrapped", "**ERROR**: RATE_LIMIT_EXCEEDED")

        result = await pool.wrap(model, priority=10, label="wrapped").async_chat("system", [])

        assert result == "**ERROR**: RATE_LIMIT_EXCEEDED"
        assert pool.concurrency_for(model) == 4

    asyncio.run(exercise())


def test_llm_call_pool_retries_after_lowering_concurrency():
    class RetryingChatModel(_ChatModel):
        def __init__(self, name: str):
            super().__init__(name)
            self.calls = 0

        async def async_chat(self, system, history, gen_conf=None, **kwargs):
            self.calls += 1
            if self.calls == 1:
                return "**ERROR**: 429 too many requests"
            return "ok after provider retry"

    async def exercise():
        changes = []
        pool = LLMCallPool(
            8,
            decrease_cooldown=0,
            rate_limit_retry_base_delay=0,
            on_concurrency_change=lambda old, new, reason: changes.append((old, new, reason)),
        )
        model = RetryingChatModel("internally-retried")

        result = await pool.wrap(model, priority=10, label="wrapped").async_chat("system", [])

        assert result == "ok after provider retry"
        assert model.calls == 2
        assert pool.concurrency_for(model) == 4
        assert changes == [(8, 4, "rate limited")]

    asyncio.run(exercise())


def test_llm_call_pool_retries_rate_limit_exceptions():
    async def exercise():
        calls = 0
        model = _ChatModel("exception-retry")
        pool = LLMCallPool(8, decrease_cooldown=0, rate_limit_retry_base_delay=0)

        async def rate_limited_once():
            nonlocal calls
            calls += 1
            if calls == 1:
                raise RuntimeError("429 too many requests")
            return "ok"

        result = await pool.call(
            rate_limited_once,
            model_key=pool._model_key(model),
            priority=10,
            label="exception-retry",
        )

        assert result == "ok"
        assert calls == 2
        assert pool.concurrency_for(model) == 4

    asyncio.run(exercise())


def test_llm_call_pool_recovers_concurrency_gradually_after_success():
    async def exercise():
        clock = _Clock()
        pool = LLMCallPool(
            8,
            decrease_cooldown=0,
            recovery_successes=2,
            recovery_cooldown=10,
            rate_limit_retries=0,
            clock=clock,
        )
        model = _ChatModel("recovering")
        model_key = pool._model_key(model)

        await pool.call(
            lambda: _return("**ERROR**: RATE_LIMIT_EXCEEDED"),
            model_key=model_key,
            priority=10,
            label="limited",
        )
        assert pool.concurrency_for(model) == 4

        clock.advance(10)
        for _ in range(2):
            await pool.call(lambda: _return("ok"), model_key=model_key, priority=10, label="success")
        assert pool.concurrency_for(model) == 5

        for _ in range(2):
            await pool.call(lambda: _return("ok"), model_key=model_key, priority=10, label="success")
        assert pool.concurrency_for(model) == 5

        clock.advance(10)
        await pool.call(lambda: _return("ok"), model_key=model_key, priority=10, label="success")
        assert pool.concurrency_for(model) == 6

    asyncio.run(exercise())


def test_llm_call_pool_reports_concurrency_changes():
    async def exercise():
        clock = _Clock()
        changes = []

        def on_change(old, new, reason):
            changes.append((old, new, reason, pool._condition.locked()))

        pool = LLMCallPool(
            8,
            decrease_cooldown=0,
            recovery_successes=1,
            recovery_cooldown=5,
            rate_limit_retries=0,
            clock=clock,
            on_concurrency_change=on_change,
        )
        model = _ChatModel("reported")
        model_key = pool._model_key(model)

        await pool.call(
            lambda: _return("**ERROR**: 429 too many requests"),
            model_key=model_key,
            priority=10,
            label="limited",
        )
        clock.advance(5)
        await pool.call(lambda: _return("ok"), model_key=model_key, priority=10, label="success")

        assert changes == [(8, 4, "rate limited", False), (4, 5, "recovered", False)]

    asyncio.run(exercise())


def test_llm_call_pool_reports_final_call_error():
    async def exercise():
        errors = []
        pool = LLMCallPool(
            2,
            rate_limit_retries=0,
            on_error=lambda label, context, error: errors.append((label, context, str(error))),
        )
        model_key = "provider/model"

        async def fail():
            raise RuntimeError("provider unavailable")

        with pytest.raises(RuntimeError, match="provider unavailable"):
            await pool.call(
                fail,
                model_key=model_key,
                priority=1,
                label="wiki-map",
                context="kb:doc",
            )

        assert errors == [("wiki-map", "kb:doc", "RuntimeError")]

    asyncio.run(exercise())


def test_llm_call_pool_reports_terminal_error_result():
    async def exercise():
        errors = []
        pool = LLMCallPool(
            2,
            rate_limit_retries=0,
            on_error=lambda label, context, error_type: errors.append((label, context, error_type)),
        )

        result = await pool.call(
            lambda: _return("**ERROR**: provider unavailable"),
            model_key="provider/model",
            priority=1,
            label="wiki-plan",
            context="kb:plan",
        )

        assert result == "**ERROR**: provider unavailable"
        assert errors == [("wiki-plan", "kb:plan", "ProviderErrorResult")]

    asyncio.run(exercise())


def test_llm_call_pool_isolates_models_and_avoids_head_of_line_blocking():
    async def exercise():
        pool = LLMCallPool(2, max_pending=4, decrease_cooldown=0, rate_limit_retries=0)
        limited_model = _ChatModel("limited")
        healthy_model = _ChatModel("healthy")
        limited_key = pool._model_key(limited_model)
        healthy_key = pool._model_key(healthy_model)

        await pool.call(
            lambda: _return("**ERROR**: 429 too many requests"),
            model_key=limited_key,
            priority=10,
            label="limited",
        )
        assert pool.concurrency_for(limited_model) == 1
        assert pool.concurrency_for(healthy_model) == 2

        release_first = asyncio.Event()
        first_started = asyncio.Event()
        second_started = asyncio.Event()
        healthy_started = asyncio.Event()

        async def first_limited_call():
            first_started.set()
            await release_first.wait()
            return "ok"

        async def second_limited_call():
            second_started.set()
            return "ok"

        async def healthy_call():
            healthy_started.set()
            return "ok"

        first = asyncio.create_task(pool.call(first_limited_call, model_key=limited_key, priority=10, label="limited-1"))
        await first_started.wait()
        second = asyncio.create_task(pool.call(second_limited_call, model_key=limited_key, priority=0, label="limited-2"))
        healthy = asyncio.create_task(pool.call(healthy_call, model_key=healthy_key, priority=20, label="healthy"))

        await asyncio.wait_for(healthy_started.wait(), timeout=1)
        assert not second_started.is_set()

        release_first.set()
        await asyncio.gather(first, second, healthy)
        assert second_started.is_set()

    asyncio.run(exercise())


def test_llm_call_pool_only_backs_off_for_rate_limit_errors():
    async def exercise():
        pool = LLMCallPool(8, decrease_cooldown=0, rate_limit_retries=0)
        model = _ChatModel("errors")
        model_key = pool._model_key(model)

        with pytest.raises(ValueError, match="invalid request"):
            await pool.call(
                lambda: _raise(ValueError("invalid request")),
                model_key=model_key,
                priority=10,
                label="invalid",
            )
        assert pool.concurrency_for(model) == 8

        with pytest.raises(RuntimeError, match="429"):
            await pool.call(
                lambda: _raise(RuntimeError("429 too many requests")),
                model_key=model_key,
                priority=10,
                label="limited",
            )
        assert pool.concurrency_for(model) == 4

    asyncio.run(exercise())


async def _raise(exc: Exception):
    raise exc
