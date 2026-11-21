import { cache } from "react";
import { loadAccount } from "@/app/actions";

// The account layout and the page inside it both need the account; this makes them share one request to the API.
export const getAccount = cache(loadAccount);
