import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, ShieldAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { adminHostsAPI } from "../../utils/api";
import { invalidateHostScope } from "../../utils/queryScopes";
import ModalPortal from "../ui/ModalPortal";

export function HostRebootBanner({ host, connected }) {
	if (!host.needs_reboot) return null;
	const reason =
		host.reboot_reason ||
		"Installed updates require a reboot to finish applying changes.";
	// Only explicit reported security context earns the label. Pending package
	// counts and kernel version mismatches do not establish security impact.
	const security =
		/^(?:reboot required to (?:complete|finish) security updates|(?:kernel )?security updates? (?:were )?(?:installed|requires? a reboot))/i.test(
			reason,
		);
	const Icon = security ? ShieldAlert : RotateCcw;
	return (
		<section
			aria-label="Reboot reason"
			className="flex items-start gap-3 rounded-lg border border-l-4 border-warning-300 bg-warning-50 dark:border-warning-700 dark:bg-warning-900/20 p-4 mb-6 text-warning-900 dark:text-warning-100"
		>
			<Icon className="h-5 w-5 shrink-0 mt-0.5" />
			<div>
				<h2 className="text-sm font-semibold flex items-center gap-2 flex-wrap">
					{security
						? "Reboot required to complete security updates"
						: "Installed updates require a reboot"}
					{security && (
						<span className="text-[10px] uppercase tracking-wide border border-warning-500 rounded px-1.5 py-0.5">
							Security
						</span>
					)}
				</h2>
				<p className="text-sm mt-1">{reason}</p>
				{!connected && (
					<p className="text-xs mt-1">
						Agent offline — waiting for the agent to reconnect.
					</p>
				)}
			</div>
		</section>
	);
}

export default function HostRebootControls({ host, connected }) {
	const client = useQueryClient();
	const [open, setOpen] = useState(false);
	const trigger = useRef(null);
	const cancel = useRef(null);
	const queryKey = ["host-reboot", host.id];
	const status = useQuery({
		queryKey,
		queryFn: () =>
			adminHostsAPI.getRebootStatus(host.id).then((res) => res.data),
		refetchInterval: 2000,
	});
	const mutation = useMutation({
		mutationFn: () => adminHostsAPI.reboot(host.id).then((res) => res.data),
		onSuccess: (request) => {
			client.setQueryData(queryKey, (old) => ({ ...old, request }));
			setOpen(false);
		},
		onSettled: () => client.invalidateQueries({ queryKey }),
	});
	const request = status.data?.request;
	const active = request && !["failed", "completed"].includes(request.status);
	const blocked = !connected
		? "Agent offline — waiting for the agent to reconnect."
		: status.isError
			? "Unable to check reboot availability. Try refreshing."
			: !status.data
				? "Checking reboot availability…"
				: !status.data.supported
					? "Update this agent to an NB build supporting reboot first."
					: status.data.patching
						? "A patch run is in progress. Try again when it finishes."
						: active || mutation.isPending
							? "A reboot request is already in progress."
							: "";
	useEffect(() => {
		if (request?.status === "completed") invalidateHostScope(client, host.id);
	}, [request?.status, client, host.id]);
	useEffect(() => {
		if (!open) return;
		cancel.current?.focus();
		return () => trigger.current?.focus();
	}, [open]);
	const error =
		mutation.error?.response?.data?.error || mutation.error?.message;
	return (
		<>
			<button
				ref={trigger}
				type="button"
				aria-label="Reboot host"
				title={blocked || "Reboot this host, even when no restart is required"}
				disabled={Boolean(blocked)}
				className="btn-outline flex items-center gap-2 text-sm whitespace-nowrap"
				onClick={() => {
					mutation.reset();
					setOpen(true);
				}}
			>
				<RotateCcw className="h-4 w-4" />
				<span>Reboot</span>
			</button>
			{request && (
				<p
					role="status"
					className={`basis-full text-xs mt-1 ${request.status === "failed" ? "text-danger-600 dark:text-danger-400" : "text-secondary-600 dark:text-secondary-300"}`}
				>
					{request.message}
				</p>
			)}
			{error && !open && (
				<p
					role="alert"
					className="basis-full text-xs text-danger-600 dark:text-danger-400"
				>
					{error}
				</p>
			)}
			{open && (
				<ModalPortal>
					<div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
						<section
							role="dialog"
							aria-modal="true"
							aria-labelledby="reboot-confirm-title"
							aria-describedby="reboot-confirm-warning"
							className="card w-full max-w-lg p-6 shadow-xl"
							onKeyDown={(event) => {
								if (event.key === "Escape" && !mutation.isPending) {
									setOpen(false);
									event.stopPropagation();
								}
								if (event.key === "Tab") {
									const buttons = [
										...event.currentTarget.querySelectorAll(
											"button:not(:disabled)",
										),
									];
									const first = buttons[0];
									const last = buttons.at(-1);
									if (event.shiftKey && document.activeElement === first) {
										event.preventDefault();
										last?.focus();
									}
									if (!event.shiftKey && document.activeElement === last) {
										event.preventDefault();
										first?.focus();
									}
								}
							}}
						>
							<RotateCcw className="h-6 w-6 text-warning-600 mb-4" />
							<h2
								id="reboot-confirm-title"
								className="text-xl font-semibold mb-3"
							>
								Reboot {host.friendly_name}?
							</h2>
							<p
								id="reboot-confirm-warning"
								className="text-sm text-secondary-600 dark:text-secondary-300"
							>
								Running services and remote sessions will be interrupted while
								this machine restarts.
							</p>
							<dl className="border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 text-sm my-5 space-y-3">
								<div className="flex justify-between gap-4">
									<dt>Host</dt>
									<dd className="text-right">
										{host.friendly_name}
										<br />
										{host.ip}
									</dd>
								</div>
								<div className="flex justify-between gap-4">
									<dt>Reason</dt>
									<dd className="text-right">
										{host.needs_reboot
											? host.reboot_reason ||
												"Installed updates require a reboot"
											: "Manual reboot — no update requires a restart"}
									</dd>
								</div>
								<div className="flex justify-between gap-4">
									<dt>Timing</dt>
									<dd>Immediately after confirmation</dd>
								</div>
							</dl>
							<p className="border-l-4 border-warning-500 bg-warning-50 dark:bg-warning-900/20 p-3 text-sm">
								Only continue if this is a safe time for downtime. This request
								cannot be undone once accepted.
							</p>
							{error && (
								<p
									role="alert"
									className="text-danger-600 dark:text-danger-400 text-sm mt-3"
								>
									{error}
								</p>
							)}
							{blocked && <p className="text-sm mt-3">{blocked}</p>}
							<div className="flex justify-end gap-3 mt-6">
								<button
									ref={cancel}
									type="button"
									className="btn-outline"
									disabled={mutation.isPending}
									onClick={() => setOpen(false)}
								>
									Cancel
								</button>
								<button
									type="button"
									className="btn-warning"
									disabled={Boolean(blocked)}
									onClick={() => mutation.mutate()}
								>
									{mutation.isPending ? "Requesting…" : "Reboot host"}
								</button>
							</div>
						</section>
					</div>
				</ModalPortal>
			)}
		</>
	);
}
