import './style.css';
import { type RuntimeTransport, setTransport } from '@wailsio/runtime';
import { mount } from 'svelte';
import App from './App.svelte';

// The Playwright suite runs the frontend on the Vite dev server with no Go
// backend. Its init script (tests/fixtures/wails-mock.ts) sets
// window.__suveMockTransport, which answers the binding calls in place of the
// Wails runtime. Production builds drop this branch.
if (import.meta.env.DEV) {
  const mock = (window as { __suveMockTransport?: RuntimeTransport }).__suveMockTransport;
  if (mock) {
    setTransport(mock);
  }
}

const target = document.getElementById('app');
if (!target) {
  throw new Error('Cannot find #app element');
}

const app = mount(App, { target });

export default app;
