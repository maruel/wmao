// Auto-resizing textarea that starts as a single line and expands vertically.
// Enter submits (via onSubmit), Shift+Enter inserts a newline.
import type { JSX } from "solid-js";

interface Props {
  value: string;
  onInput: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  disabled?: boolean;
  class?: string;
}

function autoResize(el: HTMLTextAreaElement) {
  el.style.overflow = "hidden";
  el.style.height = "auto";
  el.style.height = el.scrollHeight + "px";
  el.style.overflow = "";
}

export default function AutoResizeTextarea(props: Props) {
  let ref: HTMLTextAreaElement | undefined;

  const handleInput: JSX.EventHandler<HTMLTextAreaElement, InputEvent> = (e) => {
    props.onInput(e.currentTarget.value);
    autoResize(e.currentTarget);
  };

  const handleKeyDown: JSX.EventHandler<HTMLTextAreaElement, KeyboardEvent> = (e) => {
    if (e.key === "Enter" && !e.shiftKey && props.onSubmit) {
      e.preventDefault();
      props.onSubmit();
    }
  };

  // Reset height when value is cleared externally (e.g. after submit).
  // Solid's value binding updates the DOM, but the inline height style persists.
  const prevValue = { v: props.value };
  // Use a getter to react to prop changes in the JSX below; also reset height.
  const value = () => {
    const v = props.value;
    if (v === "" && prevValue.v !== "") {
      if (ref) ref.style.height = "";
    }
    prevValue.v = v;
    return v;
  };

  return (
    <textarea
      ref={ref}
      value={value()}
      onInput={handleInput}
      onKeyDown={handleKeyDown}
      placeholder={props.placeholder}
      disabled={props.disabled}
      class={props.class}
      rows={1}
      style={{ "field-sizing": "content" }}
    />
  );
}
