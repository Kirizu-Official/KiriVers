package official.kirizu.kirivers.client.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class ClientMatrixList {
  public List<ClientMatrixRow> matrix;
}
