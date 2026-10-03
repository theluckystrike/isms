"""Document management tests."""
import uuid

import requests


def test_list_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/all", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert isinstance(data["data"], list)


def test_search_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/search?q=security", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    results = data["data"]
    assert isinstance(results, list)
    assert len(results) > 0
    assert "document_id" in results[0]


def test_search_min_length(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/search?q=a", headers=admin_headers)
    assert r.status_code == 200
    assert len(r.json()["data"]) == 0  # too short, no results


def test_validate_documents(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/validate", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert data["valid"] is True


def test_needs_review(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/needs-review", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data
    assert isinstance(data["data"], list)


def test_recently_changed(api_url, admin_headers):
    r = requests.get(f"{api_url}/documents/changed", headers=admin_headers)
    assert r.status_code == 200
    data = r.json()
    assert "data" in data


def _top_folder_names(api_url, headers):
    r = requests.get(f"{api_url}/documents/all", headers=headers)
    assert r.status_code == 200
    return [f["name"] for f in r.json()["data"]]


def _create_folder(api_url, headers, path):
    r = requests.post(f"{api_url}/documents/folders", headers=headers, json={"path": path, "title": path})
    assert r.status_code == 201, r.text


def _delete_folder(api_url, headers, path):
    return requests.delete(f"{api_url}/documents/folders", headers=headers, params={"path": path})


def test_delete_empty_folder(api_url, admin_headers):
    """A folder created in the UI can be deleted again while it is empty (#310)."""
    name = f"tmp-folder-{uuid.uuid4().hex[:8]}"
    _create_folder(api_url, admin_headers, name)
    _create_folder(api_url, admin_headers, f"{name}/nested")
    assert name in _top_folder_names(api_url, admin_headers)

    r = _delete_folder(api_url, admin_headers, name)
    assert r.status_code == 200, r.text
    assert r.json()["commit"]
    assert name not in _top_folder_names(api_url, admin_headers)

    assert _delete_folder(api_url, admin_headers, name).status_code == 404


def test_delete_folder_with_document_is_refused(api_url, admin_headers):
    name = f"tmp-folder-{uuid.uuid4().hex[:8]}"
    doc_id = f"{name}-doc"
    _create_folder(api_url, admin_headers, name)
    r = requests.post(f"{api_url}/documents", headers=admin_headers, json={
        "folder": name, "filename": f"{doc_id}.md", "document_id": doc_id, "title": "Folder delete probe",
    })
    assert r.status_code == 201, r.text

    assert _delete_folder(api_url, admin_headers, name).status_code == 409
    assert name in _top_folder_names(api_url, admin_headers)
    assert requests.get(f"{api_url}/documents/{doc_id}/body", headers=admin_headers).status_code == 200

    # Once the document is gone the folder is empty and can go too.
    assert requests.delete(f"{api_url}/documents/{doc_id}", headers=admin_headers).status_code == 200
    assert _delete_folder(api_url, admin_headers, name).status_code == 200


def test_delete_folder_requires_manager(api_url, admin_headers, contributor_headers, reader_headers):
    name = f"tmp-folder-{uuid.uuid4().hex[:8]}"
    _create_folder(api_url, admin_headers, name)

    assert _delete_folder(api_url, contributor_headers, name).status_code == 403
    assert _delete_folder(api_url, reader_headers, name).status_code == 403
    assert name in _top_folder_names(api_url, admin_headers)

    assert _delete_folder(api_url, admin_headers, name).status_code == 200
