import { ChevronDown, Wrench, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { patchingAPI } from "../../utils/patchingApi";
import { patchErrorMessage } from "../../utils/windowsPatching";
import PatchWizard from "../PatchWizard";
import ModalPortal from "../ui/ModalPortal";

// Resolve IDs on the server, never from just the visible page's host objects.
// A session fixes IDs and mode; changing filters/selection cannot expand it.
export default function HostPatchSelected({ selectedHostIds }) {
	const [menuOpen, setMenuOpen] = useState(false);
	const [session, setSession] = useState(null);
	const [stage, setStage] = useState(null);
	const [preview, setPreview] = useState(null);
	const [error, setError] = useState(null);
	const [confirmed, setConfirmed] = useState(false);
	const [outcome, setOutcome] = useState(null);
	const root = useRef(null);
	const button = useRef(null);
	const review = useRef(null);
	const previousStage = useRef(null);
	const generation = useRef(0);
	useEffect(
		() => () => {
			generation.current += 1;
		},
		[],
	);
	useEffect(() => {
		if (!menuOpen) return;
		const outside = (event) => {
			if (!root.current?.contains(event.target)) setMenuOpen(false);
		};
		document.addEventListener("pointerdown", outside);
		root.current?.querySelector('[role="menuitem"]')?.focus();
		return () => document.removeEventListener("pointerdown", outside);
	}, [menuOpen]);
	const close = () => {
		generation.current += 1;
		setStage(null);
	};
	useEffect(() => {
		if (stage === "review") review.current?.querySelector("button")?.focus();
		if (stage === null && previousStage.current !== null)
			button.current?.focus();
		previousStage.current = stage;
	}, [stage]);
	useEffect(() => {
		if (stage !== "review") return;
		const onKeyDown = (event) => {
			if (event.key === "Escape") close();
			if (event.key === "Tab") {
				const items = [
					...review.current.querySelectorAll(
						"button:not(:disabled), input:not(:disabled), a[href]",
					),
				];
				const first = items[0],
					last = items[items.length - 1];
				if (event.shiftKey && document.activeElement === first) {
					event.preventDefault();
					last?.focus();
				}
				if (!event.shiftKey && document.activeElement === last) {
					event.preventDefault();
					first?.focus();
				}
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	});
	const start = async (mode) => {
		const id = ++generation.current;
		const next = { mode, ids: [...new Set(selectedHostIds)] };
		setSession(next);
		setPreview(null);
		setConfirmed(false);
		setError(null);
		setMenuOpen(false);
		setStage("review");
		try {
			const response = await patchingAPI.previewSelection(next.ids, mode);
			if (generation.current !== id) return;
			// Fail closed if an incomplete/mismatched preview ever reaches us.
			if (
				!Array.isArray(response.hosts) ||
				response.hosts.length !== next.ids.length ||
				new Set(response.hosts.map((h) => h.id)).size !== next.ids.length ||
				response.hosts.some((h) => !next.ids.includes(h.id))
			) {
				throw new Error("Incomplete host review; close and try again");
			}
			setPreview(response.hosts);
		} catch (err) {
			if (generation.current === id) setError(patchErrorMessage(err));
		}
	};
	const ready = (preview || []).filter((h) => h.ready);
	const skipped = (preview || []).filter((h) => !h.ready);
	const label =
		session?.mode === "security" ? "Security updates only" : "All updates";
	return (
		<>
			<div className="relative" ref={root}>
				<button
					ref={button}
					type="button"
					className="btn-primary flex items-center gap-1.5 px-3 py-2 min-h-[44px] text-xs sm:text-sm"
					disabled={selectedHostIds.length === 0 || stage !== null}
					aria-haspopup="menu"
					aria-expanded={menuOpen}
					onClick={() => setMenuOpen((open) => !open)}
				>
					<Wrench className="h-4 w-4" />
					Patch Selected
					<ChevronDown className="h-4 w-4" />
				</button>
				{menuOpen && (
					<div
						role="menu"
						aria-label="Patch update scope"
						className="absolute left-0 top-full mt-2 z-30 w-64 max-w-[calc(100vw-3rem)] rounded-lg border border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 shadow-lg p-1"
						onKeyDown={(event) => {
							if (event.key === "Escape") {
								setMenuOpen(false);
								button.current?.focus();
							}
							if (event.key === "Tab") setMenuOpen(false);
							if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
								event.preventDefault();
								const items = [
									...event.currentTarget.querySelectorAll('[role="menuitem"]'),
								];
								const index = items.indexOf(document.activeElement);
								const next =
									event.key === "Home"
										? 0
										: event.key === "End"
											? items.length - 1
											: (index +
													(event.key === "ArrowDown" ? 1 : -1) +
													items.length) %
												items.length;
								items[next]?.focus();
							}
						}}
					>
						{[
							["all", "All updates"],
							["security", "Security updates only"],
						].map(([mode, text]) => (
							<button
								key={mode}
								type="button"
								role="menuitem"
								className="block w-full rounded px-3 py-2 text-left text-sm text-secondary-900 dark:text-white hover:bg-secondary-100 dark:hover:bg-secondary-700"
								onClick={() => start(mode)}
							>
								{text}
							</button>
						))}
					</div>
				)}
			</div>
			{outcome && (
				<div
					role="status"
					className="w-full text-sm text-secondary-700 dark:text-secondary-200"
				>
					{outcome.mode === "approval"
						? "Submitted for approval"
						: "Patch runs created"}
					: {outcome.runs.length} host(s). Created does not mean completed.{" "}
					{outcome.skipped} skipped or blocked.
					<Link className="underline ml-2" to="/patching?tab=runs">
						Runs &amp; History
					</Link>
					{outcome.runs.map((run) => (
						<Link
							className="underline ml-2"
							key={run.runId}
							to={`/patching/runs/${run.runId}`}
						>
							{preview?.find((host) => host.id === run.hostId)?.friendly_name ||
								run.hostId}
						</Link>
					))}
				</div>
			)}
			{stage === "review" && (
				<ModalPortal>
					<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
						<div
							ref={review}
							role="dialog"
							aria-modal="true"
							aria-labelledby="patch-selection-title"
							className="bg-white dark:bg-secondary-800 rounded-xl shadow-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto p-5"
						>
							<div className="flex items-center justify-between gap-3">
								<h2
									id="patch-selection-title"
									className="text-lg font-semibold text-secondary-900 dark:text-white"
								>
									Review selected hosts
								</h2>
								<button
									type="button"
									onClick={close}
									aria-label="Close host review"
									className="btn-outline"
								>
									<X className="h-4 w-4" />
								</button>
							</div>
							<p className="my-3 text-sm text-secondary-600 dark:text-secondary-300">
								{label} · {session.ids.length} explicitly selected host(s). Only
								these hosts will be considered.
							</p>
							{!preview && !error && (
								<p role="status">Checking selected hosts…</p>
							)}
							{error && (
								<p role="alert" className="text-danger-600">
									{error}
								</p>
							)}
							{preview && (
								<>
									<p className="text-sm font-medium dark:text-white">
										{ready.length} ready · {skipped.length} skipped or blocked
									</p>
									<ul className="my-3 divide-y divide-secondary-200 dark:divide-secondary-600">
										{preview.map((host) => (
											<li key={host.id} className="py-3 text-sm break-words">
												<strong className="text-secondary-900 dark:text-white">
													{host.friendly_name || host.hostname || host.id}
												</strong>
												<p className="text-secondary-600 dark:text-secondary-300">
													{host.ready
														? `Ready${session.mode === "security" ? ` · ${host.package_names.length} reported security packages` : ""}`
														: host.reason}
												</p>
											</li>
										))}
									</ul>
									<p className="text-sm text-secondary-600 dark:text-secondary-300">
										{session.mode === "security"
											? "Security updates and required dependencies. Unsupported hosts never fall back to all updates."
											: "Full upgrades use each host’s existing patch behaviour; Windows scope is selected in the wizard."}{" "}
										Services may restart. Existing timing, approval, and reboot
										policy still apply.
									</p>
									<label className="flex gap-2 items-start my-4 text-sm text-secondary-900 dark:text-white">
										<input
											type="checkbox"
											checked={confirmed}
											disabled={!ready.length}
											onChange={(event) => setConfirmed(event.target.checked)}
										/>
										<span>
											I confirm the {ready.length} ready hosts; {skipped.length}{" "}
											selected hosts will not be submitted.
										</span>
									</label>
								</>
							)}
							<div className="flex flex-wrap justify-end gap-2 mt-5">
								<button type="button" onClick={close} className="btn-outline">
									Cancel
								</button>
								<button
									type="button"
									className="btn-primary"
									disabled={!confirmed || !ready.length}
									onClick={() => setStage("wizard")}
								>
									Continue to patch wizard
								</button>
							</div>
						</div>
					</div>
				</ModalPortal>
			)}
			{stage === "wizard" && (
				<PatchWizard
					isOpen
					onClose={close}
					onSuccess={(mode, details) =>
						setOutcome({ mode, runs: details.runs, skipped: skipped.length })
					}
					patchType={
						session.mode === "security" ? "patch_package" : "patch_all"
					}
					lockHosts
					lockPackages
					presetHosts={ready}
					packageNames={[]}
					packagesByHost={Object.fromEntries(
						ready.map((h) => [h.id, h.package_names || []]),
					)}
					selectionMode={session.mode}
					selectionSkipped={skipped}
				/>
			)}
		</>
	);
}
