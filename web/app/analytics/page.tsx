import { redirect } from "next/navigation";

// The analytics are the first tab of the admin dashboard; this address keeps working.
export default function Page() {
  redirect("/admin");
}
