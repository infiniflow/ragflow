import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { useNavigate } from 'umi';
import { useSelectBreadcrumbItems } from './use-navigate-to-folder';

export function FileBreadcrumb() {
  const breadcrumbItems = useSelectBreadcrumbItems();
  const navigate = useNavigate();
  return (
    <Breadcrumb>
      <BreadcrumbList>
        {breadcrumbItems.map((x, idx) => (
          <div key={x.path} className="flex items-center gap-2">
            {idx !== 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem
              key={x.path}
              onClick={() => navigate(x.path)}
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
