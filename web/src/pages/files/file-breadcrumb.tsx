import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useSelectBreadcrumbItems } from './use-navigate-to-folder';

export function FileBreadcrumb() {
  const breadcrumbItems = useSelectBreadcrumbItems();
  const { navigateToFolder } = useNavigatePage();
  return (
    <Breadcrumb>
      <BreadcrumbList>
        {breadcrumbItems.map((x, idx) => (
          <div key={x.folderId} className="flex items-center gap-2">
            {idx !== 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem
              key={x.folderId}
              onClick={navigateToFolder(x.folderId)}
              className="cursor-pointer"
            >
              {idx === breadcrumbItems.length - 1 ? (
                <BreadcrumbPage>{x.title}</BreadcrumbPage>
              ) : (
                x.title
              )}
            </BreadcrumbItem>
          </div>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
