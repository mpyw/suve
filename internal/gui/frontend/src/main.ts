import './style.css';
import { mount } from 'svelte';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) {
  throw new Error('Cannot find #app element');
}

const app = mount(App, { target });

export default app;
