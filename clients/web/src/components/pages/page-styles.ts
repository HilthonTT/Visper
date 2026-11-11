import styled from "styled-components";

/** Centred column, like the CLI's `pageContent`. */
export const PageContent = styled.section<{ $width?: string }>`
  width: min(100%, ${({ $width }) => $width ?? "36rem"});
  margin: auto;
  padding: 2.5rem 0;
`;

export const PageTitle = styled.h2`
  margin-bottom: 0.75rem;
  color: ${({ theme }) => theme.colors.text[100]};
  font-weight: 700;
`;

export const Lead = styled.p`
  margin-bottom: 1.5rem;
  color: ${({ theme }) => theme.colors.text[200]};
`;

/** Rounded box with a coloured border, like lipgloss.RoundedBorder(). */
export const Box = styled.div<{ $active?: boolean; $tone?: "danger" }>`
  padding: 0.5rem 0.9rem;
  border: 1px solid
    ${({ theme, $active, $tone }) =>
			$tone === "danger"
				? theme.colors.danger
				: $active
					? theme.colors.primary
					: theme.colors.border};
  border-radius: 8px;
  transition: border-color 120ms ease;
`;

export const Button = styled.button`
  padding: 0 0.7rem;
  border-radius: 3px;
  background: ${({ theme }) => theme.colors.primary};
  color: ${({ theme }) => theme.colors.onPrimary};
  font-weight: 700;

  &:disabled {
    opacity: 0.6;
    cursor: progress;
  }
`;
