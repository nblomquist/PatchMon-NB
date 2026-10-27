import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import HostRebootControls, {
	HostRebootBanner,
} from "../../components/host/HostRebootControls";
import { adminHostsAPI } from "../../utils/api";

vi.mock("../../utils/api", () => ({
	adminHostsAPI: { getRebootStatus: vi.fn(), reboot: vi.fn() },
}));
vi.mock("../../utils/queryScopes", () => ({ invalidateHostScope: vi.fn() }));

const host = {
	id: "h1",
	friendly_name: "web-01",
	ip: "192.0.2.11",
	needs_reboot: false,
};
let availability;
function mount(props = {}) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	});
	return render(
		<QueryClientProvider client={client}>
			<HostRebootControls host={host} connected {...props} />
		</QueryClientProvider>,
	);
}
beforeEach(() => {
	vi.resetAllMocks();
	availability = { supported: true, patching: false, request: null };
	adminHostsAPI.getRebootStatus.mockImplementation(() =>
		Promise.resolve({ data: availability }),
	);
	adminHostsAPI.reboot.mockImplementation(() => {
		availability = {
			...availability,
			request: {
				request_id: "r1",
				status: "pending",
				message: "Waiting for agent acknowledgement",
			},
		};
		return Promise.resolve({ data: availability.request });
	});
});

describe("host reboot action", () => {
	it("is enabled without a pending reboot and requires a second explicit click", async () => {
		mount();
		const trigger = screen.getByRole("button", { name: "Reboot host" });
		await waitFor(() => expect(trigger).toBeEnabled());
		fireEvent.click(trigger);
		const dialog = screen.getByRole("dialog");
		expect(
			within(dialog).getByText("Manual reboot — no update requires a restart"),
		).toBeInTheDocument();
		expect(adminHostsAPI.reboot).not.toHaveBeenCalled();
		fireEvent.click(
			within(dialog).getByRole("button", { name: "Reboot host" }),
		);
		await waitFor(() =>
			expect(adminHostsAPI.reboot).toHaveBeenCalledExactlyOnceWith("h1"),
		);
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);
		expect(trigger).toBeDisabled();
		expect(screen.getByRole("status")).toHaveTextContent(
			"Waiting for agent acknowledgement",
		);
	});
	it("allows cancellation and restores focus without sending a command", async () => {
		mount();
		const trigger = screen.getByRole("button", { name: "Reboot host" });
		await waitFor(() => expect(trigger).toBeEnabled());
		fireEvent.click(trigger);
		expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
		fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
		expect(adminHostsAPI.reboot).not.toHaveBeenCalled();
		expect(trigger).toHaveFocus();
	});
	it.each([
		"offline",
		"unsupported",
		"patching",
		"pending",
		"unknown",
		"unavailable",
	])("keeps the action visible but disabled for %s", async (state) => {
		if (state === "unsupported") availability.supported = false;
		if (state === "patching") availability.patching = true;
		if (["pending", "unknown"].includes(state))
			availability.request = { status: state, message: "Check host" };
		if (state === "unavailable")
			adminHostsAPI.getRebootStatus.mockRejectedValue(new Error("offline"));
		mount({ connected: state !== "offline" });
		await waitFor(() =>
			expect(adminHostsAPI.getRebootStatus).toHaveBeenCalled(),
		);
		expect(screen.getByRole("button", { name: "Reboot host" })).toBeDisabled();
	});
	it("shows server failures without claiming that the machine rebooted", async () => {
		adminHostsAPI.reboot.mockRejectedValue({
			response: { data: { error: "Agent is updating" } },
		});
		mount();
		const trigger = screen.getByRole("button", { name: "Reboot host" });
		await waitFor(() => expect(trigger).toBeEnabled());
		fireEvent.click(trigger);
		fireEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Reboot host",
			}),
		);
		expect(await screen.findByRole("alert")).toHaveTextContent(
			"Agent is updating",
		);
		expect(screen.getByRole("dialog")).toBeInTheDocument();
	});
});

describe("reboot reason banner", () => {
	it("does not show a banner when no restart is required", () => {
		render(<HostRebootBanner host={host} connected />);
		expect(screen.queryByRole("region")).not.toBeInTheDocument();
	});
	it("does not infer security impact from a generic kernel reason or update count", () => {
		render(
			<HostRebootBanner
				host={{
					...host,
					needs_reboot: true,
					reboot_reason: "New Linux kernel installed",
					security_updates_count: 5,
				}}
				connected
			/>,
		);
		expect(screen.getByRole("heading")).toHaveTextContent(
			"Installed updates require a reboot",
		);
		expect(screen.queryByText("Security")).not.toBeInTheDocument();
		expect(screen.queryByRole("button")).not.toBeInTheDocument();
	});
	it("emphasises explicit security context and explains offline status", () => {
		render(
			<HostRebootBanner
				host={{
					...host,
					needs_reboot: true,
					reboot_reason: "Kernel security update installed",
				}}
				connected={false}
			/>,
		);
		expect(screen.getByText("Security")).toBeInTheDocument();
		expect(
			screen.getByText("Agent offline — waiting for the agent to reconnect."),
		).toBeInTheDocument();
	});
	it("does not mistake an unclassified or negative security mention for evidence", () => {
		render(
			<HostRebootBanner
				host={{
					...host,
					needs_reboot: true,
					reboot_reason: "Kernel updated; security impact is unknown",
				}}
				connected
			/>,
		);
		expect(screen.queryByText("Security")).not.toBeInTheDocument();
	});
});
