// SPDX-License-Identifier: GPL-3.0-or-later
import { limiter } from './limiter';

function deferredTask() {
  const state = {
    started: 0,
    resolve: (): void => {},
    reject: (_e: Error): void => {},
    signal: undefined as AbortSignal | undefined,
  };
  const task = (signal: AbortSignal) => {
    state.started++;
    state.signal = signal;
    return new Promise<string>((resolve, reject) => {
      state.resolve = () => resolve('done');
      state.reject = reject;
    });
  };
  return { state, task };
}

function setup(count: number, max = 4) {
  const run = limiter(max);
  const controllers = Array.from({ length: count }, () => new AbortController());
  const tasks = Array.from({ length: count }, deferredTask);
  const results = tasks.map(({ task }, i) => run(task, controllers[i].signal));
  results.forEach((r) => r.catch(() => {}));
  const started = () => tasks.map((t) => t.state.started);
  return { tasks, controllers, results, started };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('limiter', () => {
  it('starts at most max tasks and the next one when one succeeds', async () => {
    const { tasks, started } = setup(6);
    expect(started()).toEqual([1, 1, 1, 1, 0, 0]);

    tasks[0].state.resolve();
    await tick();

    expect(started()).toEqual([1, 1, 1, 1, 1, 0]);
  });

  it('starts the next task when one fails', async () => {
    const { tasks, started, results } = setup(5);

    tasks[1].state.reject(new Error('boom'));
    await expect(results[1]).rejects.toThrow('boom');

    expect(started()).toEqual([1, 1, 1, 1, 1]);
  });

  it('rejects an aborted queued task with AbortError and never starts it', async () => {
    const { tasks, controllers, results, started } = setup(6);

    controllers[4].abort();
    await expect(results[4]).rejects.toMatchObject({ name: 'AbortError' });
    tasks[0].state.resolve();
    await tick();

    expect(started()).toEqual([1, 1, 1, 1, 0, 1]);
  });

  it('aborts the signal of a running task and starts the next one', async () => {
    const { tasks, controllers, started, results } = setup(5);

    controllers[2].abort();
    await expect(results[2]).rejects.toMatchObject({ name: 'AbortError' });
    await tick();

    expect(tasks[2].state.signal?.aborted).toBe(true);
    expect(started()).toEqual([1, 1, 1, 1, 1]);
  });

  it('rejects at once for a signal already aborted', async () => {
    const run = limiter(1);
    const controller = new AbortController();
    controller.abort();
    const { task, state } = deferredTask();

    await expect(run(task, controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(state.started).toBe(0);
  });

  it('never starts queued tasks that are aborted', async () => {
    const { tasks, controllers, started } = setup(6);

    controllers.slice(4).forEach((c) => c.abort());
    await tick();

    expect(started()).toEqual([1, 1, 1, 1, 0, 0]);
    expect(tasks[4].state.signal).toBeUndefined();
  });

  it('frees the slot of a task that throws synchronously', async () => {
    const run = limiter(1);
    const { task, state } = deferredTask();

    await expect(
      run(() => {
        throw new Error('sync boom');
      }, new AbortController().signal)
    ).rejects.toThrow('sync boom');
    run(task, new AbortController().signal);

    expect(state.started).toBe(1);
  });

  it('starts nothing for a queued task whose signal aborted before its turn', async () => {
    const run = limiter(1);
    const first = deferredTask();
    const controller = new AbortController();
    const second = deferredTask();
    const third = deferredTask();
    run(first.task, new AbortController().signal);
    run(second.task, controller.signal).catch(() => {});
    run(third.task, new AbortController().signal);

    controller.abort();
    first.state.resolve();
    await tick();

    expect(second.state.started).toBe(0);
    expect(third.state.started).toBe(1);
  });
});
