import { mount } from 'svelte';
import App from './App.svelte';
import NativeApp from './NativeApp.svelte';
import './styles.css';
mount(import.meta.env.MODE === 'native' ? NativeApp : App, { target: document.getElementById('app') });
