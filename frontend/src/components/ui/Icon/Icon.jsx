import React from 'react';
import styles from './Icon.module.css';

export function Icon({ name }) {
  const paths = {
    queue: <path d="M4 6h16M4 12h16M4 18h16" />,
    chart: <path d="M5 19V5M5 19h15M9 15v-4M13 15V8M17 15v-7" />,
    users: <path d="M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm-5 9c0-4 2-6 5-6s5 2 5 6m3-9a2.5 2.5 0 1 0 0-5m-1 14c0-2 1-4 4-4" />,
    live: <path d="M12 12m-3 0a3 3 0 1 0 6 0 3 3 0 1 0-6 0M5 12a7 7 0 0 1 14 0M2 12a10 10 0 0 1 20 0" />,
    back: <path d="M15 6l-6 6 6 6" />,
    dashboard: <path d="M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z" />,
  };

  return (
    <svg className={`${styles.root} icon`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
      {paths[name]}
    </svg>
  );
}
