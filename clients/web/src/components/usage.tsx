import { UsageDiv } from "./styles/output-styled";

const usages = {
	themes: { action: "set", placeholder: "theme-name", example: "ubuntu" },
} as const;

interface UsageProps {
	cmd: keyof typeof usages;
	marginY?: boolean;
}

export const Usage = ({ cmd, marginY }: UsageProps) => {
	const { action, placeholder, example } = usages[cmd];

	return (
		<UsageDiv data-testid={`${cmd}-invalid-arg`} $marginY={marginY}>
			Usage: {cmd} {action} &#60;{placeholder}&#62; <br />
			eg: {cmd} {action} {example}
		</UsageDiv>
	);
};
