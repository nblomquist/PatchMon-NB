import { describe, expect, it } from "vitest";
import { compareVersions } from "../../utils/version";

describe("NB build ordering", () => {
	it("preserves upstream order and compares fork build numbers numerically", () => {
		const versions = [
			"2.1.6",
			"2.1.6~nb1",
			"2.1.6~nb2",
			"2.1.6~nb10",
			"2.1.7-rc.1~nb1",
			"2.1.7",
		];
		versions.forEach((a, i) => {
			versions.forEach((b, j) => {
				expect(compareVersions(a, b)).toBe(Math.sign(i - j));
			});
		});
	});
});
