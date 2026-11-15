// SPDX-License-Identifier: GPL-3.0-or-later
export function abortError(): Error {
  const error = new Error('The request was aborted.');
  error.name = 'AbortError';
  return error;
}

type Task<T> = (signal: AbortSignal) => Promise<T>;

// limiter runs at most max tasks at once and starts the rest in call order.
export function limiter(max: number): <T>(task: Task<T>, signal: AbortSignal) => Promise<T> {
  let running = 0;
  const queue: Array<() => void> = [];

  const release = () => {
    running--;
    queue.shift()?.();
  };

  return <T>(task: Task<T>, signal: AbortSignal) =>
    new Promise<T>((resolve, reject) => {
      if (signal.aborted) {
        reject(abortError());
        return;
      }

      let released = false;
      const finish = () => {
        if (!released) {
          released = true;
          signal.removeEventListener('abort', onAbort);
          release();
        }
      };

      const start = () => {
        running++;
        if (signal.aborted) {
          finish();
          reject(abortError());
          return;
        }
        let result: Promise<T>;
        try {
          result = task(signal);
        } catch (error) {
          result = Promise.reject(error);
        }
        result.then(
          (value) => {
            finish();
            resolve(value);
          },
          (error) => {
            finish();
            reject(error);
          }
        );
      };

      const onAbort = () => {
        const queued = queue.indexOf(start);
        if (queued >= 0) {
          queue.splice(queued, 1);
          signal.removeEventListener('abort', onAbort);
        } else if (!released) {
          released = true;
          signal.removeEventListener('abort', onAbort);
          queueMicrotask(release);
        }
        reject(abortError());
      };

      signal.addEventListener('abort', onAbort);
      if (running < max) {
        start();
      } else {
        queue.push(start);
      }
    });
}
