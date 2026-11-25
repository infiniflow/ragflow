/**
 * Enterprise route seam (open-source side).
 *
 * The enterprise build overlays this file with its own implementation, which
 * merges the enterprise route tree into the tree upstream already built.
 * Open source renders the upstream router unchanged.
 */
import { routers } from '@/routes';

export { routers };
