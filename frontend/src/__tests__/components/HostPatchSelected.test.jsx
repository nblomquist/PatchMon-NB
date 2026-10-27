import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { wizard } = vi.hoisted(() => ({ wizard: vi.fn() }));
vi.mock("../../components/PatchWizard", () => ({
	default: (props) => {
		wizard(props);
		return <div>Existing patch wizard</div>;
	},
}));
vi.mock("../../utils/patchingApi", () => ({
	patchingAPI: { previewSelection: vi.fn() },
}));

import HostPatchSelected from "../../components/host/HostPatchSelected";
import { patchingAPI } from "../../utils/patchingApi";

const hosts = [
	{
		id: "off-page",
		friendly_name: "web-01",
		ready: true,
		package_names: ["openssl"],
	},
	{ id: "db", friendly_name: "db-01", ready: true, package_names: ["sudo"] },
	{
		id: "offline",
		friendly_name: "edge-01",
		ready: false,
		reason: "Agent offline — waiting for the agent to reconnect.",
	},
];
const view = (ids = hosts.map((h) => h.id)) =>
	render(
		<MemoryRouter>
			<HostPatchSelected selectedHostIds={ids} />
		</MemoryRouter>,
	);
const open = (name = "Security updates only") => {
	fireEvent.click(screen.getByRole("button", { name: "Patch Selected" }));
	fireEvent.click(screen.getByRole("menuitem", { name }));
};

describe("Patch Selected host review", () => {
	it("links created runs to their details and the existing Runs & History tab", async () => {
		view();
		open();
		await screen.findByText("2 ready · 1 skipped or blocked");
		fireEvent.click(screen.getByRole("checkbox"));
		fireEvent.click(
			screen.getByRole("button", { name: "Continue to patch wizard" }),
		);
		act(() => {
			wizard.mock.lastCall[0].onSuccess("patch", {
				runs: [{ hostId: "off-page", runId: "run-web" }],
			});
			wizard.mock.lastCall[0].onClose();
		});
		expect(
			screen.getByRole("link", { name: "Runs & History" }),
		).toHaveAttribute("href", "/patching?tab=runs");
		expect(screen.getByRole("link", { name: "web-01" })).toHaveAttribute(
			"href",
			"/patching/runs/run-web",
		);
	});
	beforeEach(() => {
		vi.clearAllMocks();
		patchingAPI.previewSelection.mockResolvedValue({ hosts });
	});
	it("uses explicit IDs including off-page selection, shows exclusions, and fixes per-host security scope", async () => {
		const { rerender } = view();
		open();
		await screen.findByText("2 ready · 1 skipped or blocked");
		expect(patchingAPI.previewSelection).toHaveBeenCalledWith(
			["off-page", "db", "offline"],
			"security",
		);
		expect(screen.getByText(hosts[2].reason)).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Continue to patch wizard" }),
		).toBeDisabled();
		rerender(
			<MemoryRouter>
				<HostPatchSelected selectedHostIds={["other"]} />
			</MemoryRouter>,
		);
		fireEvent.click(screen.getByRole("checkbox"));
		fireEvent.click(
			screen.getByRole("button", { name: "Continue to patch wizard" }),
		);
		expect(wizard.mock.lastCall[0]).toMatchObject({
			patchType: "patch_package",
			selectionMode: "security",
			lockHosts: true,
			lockPackages: true,
			presetHosts: hosts.slice(0, 2),
			packagesByHost: { "off-page": ["openssl"], db: ["sudo"] },
			selectionSkipped: [hosts[2]],
		});
	});
	it("opens the all-updates wizard without executing from the dropdown", async () => {
		view();
		open("All updates");
		await screen.findByText("2 ready · 1 skipped or blocked");
		expect(wizard).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole("checkbox"));
		fireEvent.click(
			screen.getByRole("button", { name: "Continue to patch wizard" }),
		);
		expect(wizard.mock.lastCall[0]).toMatchObject({
			patchType: "patch_all",
			selectionMode: "all",
		});
	});
	it("cancel sends no patch work and returns focus", async () => {
		view();
		open();
		await screen.findByText("2 ready · 1 skipped or blocked");
		fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
		expect(wizard).not.toHaveBeenCalled();
		expect(
			screen.getByRole("button", { name: "Patch Selected" }),
		).toHaveFocus();
	});
	it("blocks empty or all-blocked selections", async () => {
		const { unmount } = view([]);
		expect(
			screen.getByRole("button", { name: "Patch Selected" }),
		).toBeDisabled();
		unmount();
		patchingAPI.previewSelection.mockResolvedValue({ hosts: [hosts[2]] });
		view(["offline"]);
		open();
		await screen.findByText("0 ready · 1 skipped or blocked");
		expect(screen.getByRole("checkbox")).toBeDisabled();
		expect(
			screen.getByRole("button", { name: "Continue to patch wizard" }),
		).toBeDisabled();
	});
	it("rejects a mismatched server preview without adding targets", async () => {
		patchingAPI.previewSelection.mockResolvedValue({ hosts: [hosts[0]] });
		view();
		open();
		await screen.findByRole("alert");
		expect(wizard).not.toHaveBeenCalled();
	});
	it("supports keyboard scope selection and Escape", async () => {
		view();
		fireEvent.click(screen.getByRole("button", { name: "Patch Selected" }));
		expect(screen.getByRole("menuitem", { name: "All updates" })).toHaveFocus();
		fireEvent.keyDown(screen.getByRole("menu"), { key: "ArrowDown" });
		expect(
			screen.getByRole("menuitem", { name: "Security updates only" }),
		).toHaveFocus();
		fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
		await waitFor(() =>
			expect(screen.queryByRole("menu")).not.toBeInTheDocument(),
		);
		expect(
			screen.getByRole("button", { name: "Patch Selected" }),
		).toHaveFocus();
	});
});
