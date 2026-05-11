import styles from './StateMessage.module.css';

export function StateMessage({ title, text }) {
  return (
    <section className="content">
      <div className={`${styles.root} state-message`}>
        <h1>{title}</h1>
        <p>{text}</p>
      </div>
    </section>
  );
}
