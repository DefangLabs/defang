import { LoaderFunctionArgs, json } from "@remix-run/node";
import { useLoaderData } from "@remix-run/react";
import { prisma } from "~/client";

export async function loader({}: LoaderFunctionArgs) {
  const notes = await prisma.note.findMany();
  return json(notes);
}

export default function NotesIndex() {
  const data = useLoaderData<typeof loader>();
  return (
    <div>
      <ul>
        {data.map((note) => (
          <li key={note.id}>
            <a href={`/notes/${note.id}`}>{note.title}</a>
          </li>
        ))}
      </ul>
    </div>
  );
}
