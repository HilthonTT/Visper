import styled from "styled-components";

/** Clickable command pill: runs its command when pressed. */
export const Chip = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.05rem 0.6rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 6px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
  color: ${({ theme }) => theme.colors.primary};
  line-height: 1.6;
  white-space: nowrap;
  transition: border-color 120ms ease, background-color 120ms ease;

  &:hover {
    border-color: ${({ theme }) => theme.colors.primary};
    background: color-mix(in srgb, ${({ theme }) => theme.colors.primary} 10%, ${({ theme }) => theme.colors.surfaceAlt});
  }
`;

export const ChipRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
`;

export const Kbd = styled.kbd`
  display: inline-block;
  min-width: 1.6em;
  padding: 0 0.4rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-bottom-width: 2px;
  border-radius: 5px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
  color: ${({ theme }) => theme.colors.text[100]};
  font-family: inherit;
  font-size: 0.85em;
  line-height: 1.5;
  text-align: center;
`;

export const Muted = styled.span`
  color: ${({ theme }) => theme.colors.text[300]};
`;

export const Subtle = styled.span`
  color: ${({ theme }) => theme.colors.text[200]};
`;

export const Accent = styled.span`
  color: ${({ theme }) => theme.colors.primary};
`;

export const Strong = styled.span`
  color: ${({ theme }) => theme.colors.text[100]};
  font-weight: 700;
`;

/** Small uppercase label above a group of output. */
export const SectionLabel = styled.div`
  margin-bottom: 0.4rem;
  color: ${({ theme }) => theme.colors.text[300]};
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

export const Panel = styled.div`
  padding: 0.9rem 1.1rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 10px;
  background: ${({ theme }) => theme.colors.surfaceAlt};
`;
