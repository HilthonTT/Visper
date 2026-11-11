import styled from "styled-components";
import { faq } from "@/lib/faq";
import { Lead, PageContent, PageTitle } from "./page-styles";

export const FaqPage = () => (
	<PageContent $width="44rem">
		<PageTitle>Frequently asked questions</PageTitle>
		<Lead>How Visper works, in short.</Lead>
		<List>
			{faq.map(({ question, answer }, i) => (
				<Item key={question} open={i === 0}>
					<summary>{question}</summary>
					<p>{answer}</p>
				</Item>
			))}
		</List>
	</PageContent>
);

const List = styled.div`
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
`;

const Item = styled.details`
  padding: 0.45rem 0.9rem;
  border: 1px solid ${({ theme }) => theme.colors.border};
  border-radius: 8px;

  &[open] {
    border-color: ${({ theme }) => theme.colors.primary};
  }

  summary {
    list-style: none;
    cursor: pointer;
    color: ${({ theme }) => theme.colors.text[100]};
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary::before {
    content: "▸ ";
    color: ${({ theme }) => theme.colors.primary};
  }
  &[open] summary::before {
    content: "▾ ";
  }
  &[open] summary {
    font-weight: 700;
  }

  p {
    margin: 0.35rem 0 0.15rem 1.2rem;
    color: ${({ theme }) => theme.colors.text[200]};
  }
`;
