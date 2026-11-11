import { useContext } from "react";
import styled, { type DefaultTheme, useTheme } from "styled-components";
import { useShell } from "@/contexts/shell-context";
import { themeContext } from "@/hooks/use-theme";
import { companions, getCompanion } from "@/lib/companions";
import themes, { themeNames } from "@/lib/themes";
import { Lead, PageContent, PageTitle } from "./page-styles";

export const SettingsPage = () => {
	const { companionId, setCompanionId } = useShell();
	const switchTheme = useContext(themeContext);
	const active = useTheme();
	const selected = getCompanion(companionId);

	return (
		<PageContent $width="54rem">
			<Layout>
				<div>
					<PageTitle>Chat companion</PageTitle>
					<Lead>Who keeps you company in the side panel.</Lead>
					<Options role="radiogroup" aria-label="Companion">
						{companions.map((c) => (
							<Option
								key={c.id}
								type="button"
								role="radio"
								aria-checked={c.id === selected.id}
								onClick={() => setCompanionId(c.id)}
							>
								<OptionHead>
									<Radio>{c.id === selected.id ? "●" : "○"}</Radio>
									<OptionTitle>{c.title}</OptionTitle>
									<OptionNum>{c.id}</OptionNum>
								</OptionHead>
								<OptionDesc>{c.description}</OptionDesc>
							</Option>
						))}
					</Options>

					<Section>
						<PageTitle>Theme</PageTitle>
						<Swatches>
							{themeNames.map((name) => (
								<Swatch
									key={name}
									type="button"
									$preview={themes[name]}
									aria-pressed={active.name === name}
									onClick={() => switchTheme?.(themes[name])}
								>
									<i />
									<i />
									{name}
								</Swatch>
							))}
						</Swatches>
					</Section>
				</div>

				<Preview>
					<img src={selected.src} alt={selected.title} />
				</Preview>
			</Layout>
		</PageContent>
	);
};

const Layout = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 1fr) 16rem;
  gap: 2.5rem;
  align-items: start;

  @media (max-width: 760px) {
    grid-template-columns: minmax(0, 1fr);
  }
`;

const Options = styled.div`
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
`;

const Option = styled.button`
  display: block;
  width: 100%;
  padding: 0.45rem 0.9rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 8px;
  text-align: left;

  &:hover,
  &[aria-checked="true"] {
    border-color: ${({ theme }) => theme.colors.primary};
  }
`;

const OptionHead = styled.div`
  display: flex;
  gap: 0.6rem;
`;

const Radio = styled.span`
  color: ${({ theme }) => theme.colors.secondary};
`;

const OptionTitle = styled.span`
  color: ${({ theme }) => theme.colors.text[100]};

  [aria-checked="true"] & {
    font-weight: 700;
  }
`;

const OptionNum = styled.span`
  margin-left: auto;
  color: ${({ theme }) => theme.colors.text[300]};
`;

const OptionDesc = styled.div`
  padding-left: 1.6rem;
  color: ${({ theme }) => theme.colors.text[200]};
`;

const Section = styled.div`
  margin-top: 2rem;
`;

const Swatches = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
`;

/* Each swatch previews its own theme, not the active one. */
const Swatch = styled.button<{ $preview: DefaultTheme }>`
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.15rem 0.7rem;
  border: 1px solid ${({ $preview }) => $preview.colors.border};
  border-radius: 6px;
  background: ${({ $preview }) => $preview.colors.surface};
  color: ${({ $preview }) => $preview.colors.text[100]};

  &[aria-pressed="true"] {
    border-color: ${({ $preview }) => $preview.colors.primary};
    box-shadow: 0 0 0 1px ${({ $preview }) => $preview.colors.primary};
  }

  i {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
  i:nth-of-type(1) { background: ${({ $preview }) => $preview.colors.primary}; }
  i:nth-of-type(2) { background: ${({ $preview }) => $preview.colors.secondary}; }
`;

const Preview = styled.div`
  padding: 0.75rem;
  border: 1px solid ${({ theme }) => theme.colors.secondary};
  border-radius: 8px;

  img {
    width: 100%;
    height: auto;
  }

  @media (max-width: 760px) {
    max-width: 14rem;
  }
`;
