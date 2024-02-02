import { LoaderFunctionArgs, json } from "@remix-run/node";
import { Link, useLoaderData } from "@remix-run/react";
import { prisma } from "~/client";

export async function loader({ params }: LoaderFunctionArgs) {
  const note = await prisma.note.findUnique({
    where: { id: Number(params.id) },
  });

  if (!note) {
    throw new Response("Not found", { status: 404, statusText: "Not found" });
  }

  return json(note);
}

export default function Note() {
  const note = useLoaderData<typeof loader>();

  return (
    <div>
      <Link to="/notes">All notes</Link>
      <h2>{note.title}</h2>
      <p>{note.body}</p>
    </div>
  );
}
