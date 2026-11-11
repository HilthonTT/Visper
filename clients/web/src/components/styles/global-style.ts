import { createGlobalStyle } from "styled-components";
import { normalize } from "styled-normalize";

const GlobalStyle = createGlobalStyle`
  ${normalize}

  *, ::before, ::after {
    box-sizing: border-box;
    border-width: 0;
    border-style: solid;
    border-color: currentColor;
  }

  blockquote, dl, dd, h1, h2, h3,
  h4, h5, h6, hr, figure, p, pre {
    margin: 0;
  }

  h1, h2, h3, h4, h5, h6 {
    font-size: inherit;
    font-weight: inherit;
  }

  img, svg, video, canvas, audio,
  iframe, embed, object {
    display: block;
  }

  html, body {
    height: 100%;
  }

  body {
    font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 14px;
    font-weight: 400;
    line-height: 1.65;
    font-variant-ligatures: none;
    background-color: ${({ theme }) => theme.colors.body};
    color: ${({ theme }) => theme.colors.text[100]};
    overflow: hidden;
    transition: background-color 200ms ease, color 200ms ease;
  }

  ::selection {
    background: color-mix(in srgb, ${({ theme }) => theme.colors.primary} 30%, transparent);
  }

  /* ===== Scrollbar ===== */
  * {
    scrollbar-width: thin;
    scrollbar-color: ${({ theme }) => theme.colors.scrollHandle} transparent;
  }
  ::-webkit-scrollbar {
    width: 10px;
    height: 10px;
  }
  ::-webkit-scrollbar-track {
    background: transparent;
  }
  ::-webkit-scrollbar-thumb {
    background: ${({ theme }) => theme.colors.scrollHandle};
    border: 3px solid transparent;
    border-radius: 999px;
    background-clip: padding-box;
  }
  ::-webkit-scrollbar-thumb:hover {
    background-color: ${({ theme }) => theme.colors.scrollHandleHover};
  }

  input[type=text] {
    font: inherit;
    background: transparent;
    color: ${({ theme }) => theme.colors.text[100]};
    caret-color: ${({ theme }) => theme.colors.primary};
  }
  input[type=text]::placeholder {
    color: ${({ theme }) => theme.colors.text[300]};
  }
  input[type=text]:focus-visible {
    outline: none;
  }

  button {
    font: inherit;
    color: inherit;
    background: none;
    cursor: pointer;
  }

  :focus-visible {
    outline: 2px solid ${({ theme }) => theme.colors.primary};
    outline-offset: 2px;
  }

  @media (prefers-reduced-motion: reduce) {
    *, ::before, ::after {
      transition: none !important;
      animation: none !important;
    }
  }

  .sr-only {
    position: absolute;
    left: -10000px;
    top: auto;
    width: 1px;
    height: 1px;
    overflow: hidden;
  }
`;

export default GlobalStyle;
