import styles from './Timeline.module.css';

export function Timeline({ data }) {
  const max = Math.max(1, ...data);

  return (
    <div className={`${styles.root} timeline`}>
      {data.map((value, index) => (
        <span key={index} style={{ height: `${Math.max(4, (value / max) * 110)}px` }} />
      ))}
    </div>
  );
}
