"use client";

import type { ReactNode } from "react";
import { DashboardDataProvider } from "./DashboardDataProvider";

type AppProvidersProps = {
  children: ReactNode;
};

export default function AppProviders({ children }: AppProvidersProps) {
  return <DashboardDataProvider>{children}</DashboardDataProvider>;
}
