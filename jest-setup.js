// SPDX-License-Identifier: GPL-3.0-or-later
// Jest setup provided by Grafana scaffolding
import './.config/jest-setup';
import { MessageChannel as NodeMessageChannel } from 'worker_threads';

// Ports of a worker_threads MessageChannel keep the Node process alive, so they are closed after the tests.
const channels = [];
global.MessageChannel = class extends NodeMessageChannel {
  constructor() {
    super();
    channels.push(this);
  }
};

afterAll(() => {
  for (const { port1, port2 } of channels) {
    port1.close();
    port2.close();
  }
});
