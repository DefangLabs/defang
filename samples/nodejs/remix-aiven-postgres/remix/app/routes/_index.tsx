import { LoaderFunctionArgs, redirect } from "@remix-run/node";
import { useNavigate } from "@remix-run/react";
import { useEffect } from "react";

export async function loader({}: LoaderFunctionArgs) {
  return redirect("/notes");
}

export default function NotesIndex() {
  const navigate = useNavigate();
  useEffect(() => {
    navigate("/notes");
  }, [navigate]);

  return null;
}
