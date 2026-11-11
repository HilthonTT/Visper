import styled, { css } from "styled-components";

const MOBILE = "@media (max-width: 640px)";

/* ===== Terminal body ===== */

export const Wrapper = styled.div`
  flex: 1;
  min-height: 0;
  padding: 1.25rem 0 1rem;
  display: flex;
  flex-direction: column-reverse;
  overflow-y: auto;
  overflow-wrap: anywhere;

  ${MOBILE} {
    padding: 1rem 0;
  }
`;

/**
 * First child of the column-reversed Wrapper, so it sits visually at the
 * bottom: soaks up spare height so output starts at the top of the window,
 * and collapses to nothing once the output overflows.
 */
export const Filler = styled.div`
  flex: 1 1 0;
`;

export const CmdNotFound = styled.div`
  margin-top: 0.25rem;
  margin-bottom: 0.9rem;
  color: ${({ theme }) => theme.colors.danger};
`;

export const Empty = styled.div`
  margin-bottom: 0.25rem;
`;

const inputLine = css`
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  column-gap: 0.6rem;
`;

/** A submitted command: prompt followed by what was typed. */
export const InputLine = styled.div`
  ${inputLine}
`;

export const Form = styled.form`
  ${inputLine}
  padding: 0.15rem 0;
`;

export const Input = styled.input`
  flex: 1;
  min-width: 8rem;
  padding: 0;
  border: none;
`;

export const Hints = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem 1rem;
  margin-top: 0.25rem;
  color: ${({ theme }) => theme.colors.text[200]};
`;

/* ===== Prompt ===== */

export const Prompt = styled.span`
  flex: none;
  white-space: nowrap;
  color: ${({ theme }) => theme.colors.text[300]};
`;

export const PromptUser = styled.span`
  color: ${({ theme }) => theme.colors.secondary};
`;

export const PromptHost = styled.span`
  color: ${({ theme }) => theme.colors.primary};
`;

export const PromptPath = styled.span`
  color: ${({ theme }) => theme.colors.text[200]};
`;

export const PromptArrow = styled.span`
  color: ${({ theme }) => theme.colors.primary};
  font-weight: 700;
`;
