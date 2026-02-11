import { Show, splitProps, type JSX } from "solid-js";
import styles from "./Button.module.css";

type Variant = "primary" | "gray" | "red" | "green";

type ButtonProps = JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  loading?: boolean;
};

export default function Button(props: ButtonProps) {
  const [local, rest] = splitProps(props, ["variant", "class", "loading", "disabled", "children"]);
  const variant = () => local.variant ?? "primary";
  return (
    <button
      class={`${styles.btn} ${styles[variant()]}${local.class ? ` ${local.class}` : ""}`}
      disabled={local.disabled || local.loading}
      {...rest}
    >
      <Show when={local.loading}>
        <span class={styles.spinner} />
        {" "}
      </Show>
      {local.children}
    </button>
  );
}
