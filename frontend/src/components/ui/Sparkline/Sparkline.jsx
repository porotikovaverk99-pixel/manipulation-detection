import styles from './Sparkline.module.css';

export function Sparkline({ data }) {
  const max = Math.max(1, ...data);
  const points = data
    .map((value, index) => {
      const x = (index / Math.max(1, data.length - 1)) * 100;
      const y = 24 - (value / max) * 22;
      return `${x},${y}`;
    })
    .join(' ');

  return (
    <svg className={`${styles.root} sparkline`} viewBox="0 0 100 26" preserveAspectRatio="none">
      <polyline points={points} />
    </svg>
  );
}
