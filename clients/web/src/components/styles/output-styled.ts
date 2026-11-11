import styled, { type DefaultTheme } from "styled-components";

export const OutputContainer = styled.div`
  padding-bottom: 0.25rem;
`;

export const Wrapper = styled.div`
  margin-top: 0.25rem;
  margin-bottom: 0.9rem;
`;

export const UsageDiv = styled.div`
  margin-top: 0.25rem;
  margin-bottom: 0.9rem;
  color: ${({ theme }) => theme.colors.text[200]};
`;

export type Status = "loading" | "success" | "error";

const statusColor = (theme: DefaultTheme, status: Status) =>
	({
		loading: theme.colors.warning,
		success: theme.colors.success,
		error: theme.colors.danger,
	})[status];

/** Output line coloured by outcome, with a leading status glyph. */
export const StatusLine = styled.div<{ $status: Status }>`
  display: flex;
  gap: 0.6rem;
  margin-top: 0.25rem;
  margin-bottom: 0.9rem;
  color: ${({ theme, $status }) => statusColor(theme, $status)};

  &::before {
    content: "${({ $status }) => ({ loading: "…", success: "✓", error: "✗" })[$status]}";
    flex: none;
    font-weight: 700;
  }
`;
